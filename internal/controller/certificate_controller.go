/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/cloudflare/cloudflare-go"

	acmev1 "github.com/cert-manager/cert-manager/pkg/apis/acme/v1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	certificatev1alpha1 "github.com/tae2089/certificate-operator/api/v1alpha1"
)

const (
	certificateFinalizer = "certificate.println.kr/finalizer"
)

// CertificateReconciler reconciles a Certificate object
type CertificateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cert certificatev1alpha1.Certificate
	if err := r.Get(ctx, req.NamespacedName, &cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the Certificate is being deleted
	if !cert.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(&cert, certificateFinalizer) {
			// Run finalization logic
			if err := r.finalizeCertificate(ctx, &cert); err != nil {
				// If finalization fails, return error so it can be retried
				log.Error(err, "Failed to finalize Certificate")
				return ctrl.Result{}, err
			}

			// Remove finalizer once cleanup is done
			controllerutil.RemoveFinalizer(&cert, certificateFinalizer)
			if err := r.Update(ctx, &cert); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the object is being deleted
		return ctrl.Result{}, nil
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(&cert, certificateFinalizer) {
		controllerutil.AddFinalizer(&cert, certificateFinalizer)
		if err := r.Update(ctx, &cert); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Define Issuer
	issuerName := cert.Spec.IssuerName
	if issuerName == "" {
		issuerName = cert.Name + "-issuer"
	}

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuerName,
			Namespace: cert.Namespace,
		},
	}

	// Create or Update Issuer
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, issuer, func() error {
		if issuer.Labels == nil {
			issuer.Labels = make(map[string]string)
		}
		issuer.Labels["app.kubernetes.io/managed-by"] = "certificate-operator"

		// Set owner reference
		if err := ctrl.SetControllerReference(&cert, issuer, r.Scheme); err != nil {
			return err
		}

		// Default to ACME Staging for now
		ingressClass := cert.Spec.IngressClassName
		if ingressClass == "" {
			ingressClass = "nginx"
		}

		issuer.Spec = certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				ACME: &acmev1.ACMEIssuer{
					Email:  cert.Spec.Email,
					Server: "https://acme-staging-v02.api.letsencrypt.org/directory",
					PrivateKey: cmmeta.SecretKeySelector{
						LocalObjectReference: cmmeta.LocalObjectReference{
							Name: issuerName + "-account-key",
						},
					},
					Solvers: []acmev1.ACMEChallengeSolver{
						{
							HTTP01: &acmev1.ACMEChallengeSolverHTTP01{
								Ingress: &acmev1.ACMEChallengeSolverHTTP01Ingress{
									Class: &ingressClass,
								},
							},
						},
					},
				},
			},
		}
		return nil
	}); err != nil {
		log.Error(err, "Failed to create or update Issuer")
		return ctrl.Result{}, err
	}

	// 2. Define Certificate
	certReq := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cert.Name + "-cert",
			Namespace: cert.Namespace,
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, certReq, func() error {
		if certReq.Labels == nil {
			certReq.Labels = make(map[string]string)
		}
		certReq.Labels["app.kubernetes.io/managed-by"] = "certificate-operator"

		if err := ctrl.SetControllerReference(&cert, certReq, r.Scheme); err != nil {
			return err
		}

		certReq.Spec = certmanagerv1.CertificateSpec{
			DNSNames:   []string{cert.Spec.Domain},
			SecretName: cert.Name + "-tls",
			IssuerRef: cmmeta.ObjectReference{
				Name: issuerName,
				Kind: "Issuer",
			},
		}
		return nil
	}); err != nil {
		log.Error(err, "Failed to create or update Certificate")
		return ctrl.Result{}, err
	}

	// 3. Update Status
	statusUpdated := false
	if cert.Status.IssuerRef != issuerName || cert.Status.CertificateRef != certReq.Name {
		cert.Status.IssuerRef = issuerName
		cert.Status.CertificateRef = certReq.Name
		statusUpdated = true
	}

	// 4. Check if Issuer is Ready
	issuerReady := false
	for _, cond := range issuer.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			issuerReady = true
			break
		}
	}

	if !issuerReady {
		log.Info("Waiting for Issuer to be ready", "issuer", issuerName)
		if statusUpdated {
			if err := r.Status().Update(ctx, &cert); err != nil {
				log.Error(err, "Failed to update Certificate status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// 5. Check if Certificate is Ready
	certReady := false
	for _, cond := range certReq.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			certReady = true
			break
		}
	}

	if !certReady {
		log.Info("Waiting for Certificate to be ready", "certificate", certReq.Name)
		if statusUpdated {
			if err := r.Status().Update(ctx, &cert); err != nil {
				log.Error(err, "Failed to update Certificate status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// 6. Retrieve TLS Secret
	tlsSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cert.Name + "-tls",
		Namespace: cert.Namespace,
	}, tlsSecret); err != nil {
		log.Error(err, "Failed to retrieve TLS secret")
		if statusUpdated {
			if err := r.Status().Update(ctx, &cert); err != nil {
				log.Error(err, "Failed to update Certificate status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	tlsCert := tlsSecret.Data["tls.crt"]
	tlsKey := tlsSecret.Data["tls.key"]

	if len(tlsCert) == 0 || len(tlsKey) == 0 {
		log.Info("TLS secret is empty, waiting...", "secret", tlsSecret.Name)
		if statusUpdated {
			if err := r.Status().Update(ctx, &cert); err != nil {
				log.Error(err, "Failed to update Certificate status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// 7. Upload to Cloudflare if configured and enabled
	cloudflareEnabled := cert.Spec.CloudflareEnabled == nil || *cert.Spec.CloudflareEnabled
	if cert.Spec.CloudflareSecretRef != "" && cloudflareEnabled && !cert.Status.CloudflareUploaded {
		if err := r.uploadToCloudflare(ctx, &cert, tlsCert, tlsKey); err != nil {
			log.Error(err, "Failed to upload to Cloudflare")
		} else {
			cert.Status.CloudflareUploaded = true
			// CloudflareCertificateID is set inside uploadToCloudflare
			statusUpdated = true
			log.Info("Successfully uploaded certificate to Cloudflare", "id", cert.Status.CloudflareCertificateID)
		}
	}

	// 8. Upload to AWS ACM if configured and enabled
	awsEnabled := cert.Spec.AWSEnabled == nil || *cert.Spec.AWSEnabled
	if awsEnabled && !cert.Status.AWSUploaded {
		// Upload if awsSecretRef is specified (static credentials)
		if cert.Spec.AWSSecretRef != "" {
			arn, err := r.uploadToAWS(ctx, &cert, tlsCert, tlsKey)
			if err != nil {
				log.Error(err, "Failed to upload to AWS")
			} else {
				cert.Status.AWSUploaded = true
				cert.Status.AWSCertificateARN = arn
				statusUpdated = true
				log.Info("Successfully uploaded certificate to AWS ACM", "arn", arn)
			}
		}
	}

	// Update status if changed
	if statusUpdated {
		if err := r.Status().Update(ctx, &cert); err != nil {
			log.Error(err, "Failed to update Certificate status")
			return ctrl.Result{}, err
		}
	}

	// Requeue every minute to check for updates
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *CertificateReconciler) uploadToCloudflare(ctx context.Context, cert *certificatev1alpha1.Certificate, tlsCert, tlsKey []byte) error {
	// Get Cloudflare credentials
	cfSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cert.Spec.CloudflareSecretRef,
		Namespace: cert.Namespace,
	}, cfSecret); err != nil {
		return fmt.Errorf("failed to get Cloudflare secret: %w", err)
	}

	apiToken := string(cfSecret.Data["api-token"])
	if apiToken == "" {
		return fmt.Errorf("api-token not found in Cloudflare secret")
	}

	if cert.Spec.CloudflareZoneID == "" {
		return fmt.Errorf("cloudflareZoneID is required but not set")
	}

	// Create Cloudflare client
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Upload custom SSL certificate to Cloudflare using zone ID
	sslCert, err := api.CreateSSL(ctx, cert.Spec.CloudflareZoneID, cloudflare.ZoneCustomSSLOptions{
		Certificate: string(tlsCert),
		PrivateKey:  string(tlsKey),
	})
	if err != nil {
		return fmt.Errorf("failed to upload certificate to Cloudflare: %w", err)
	}

	// Store the certificate ID for later deletion
	cert.Status.CloudflareCertificateID = sslCert.ID

	return nil
}

func (r *CertificateReconciler) uploadToAWS(ctx context.Context, cert *certificatev1alpha1.Certificate, tlsCert, tlsKey []byte) (string, error) {
	var cfg aws.Config
	var err error

	// If awsSecretRef is empty, use default credential chain (IRSA, Instance Profile, etc.)
	if cert.Spec.AWSSecretRef == "" {
		log := logf.FromContext(ctx)
		log.Info("Using AWS default credential chain (IRSA/Instance Profile)")

		cfg, err = config.LoadDefaultConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to load AWS default config: %w", err)
		}
	} else {
		// Get AWS credentials from Secret
		awsSecret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      cert.Spec.AWSSecretRef,
			Namespace: cert.Namespace,
		}, awsSecret); err != nil {
			return "", fmt.Errorf("failed to get AWS secret: %w", err)
		}

		accessKeyID := string(awsSecret.Data["access-key-id"])
		secretAccessKey := string(awsSecret.Data["secret-access-key"])
		region := string(awsSecret.Data["region"])

		if accessKeyID == "" || secretAccessKey == "" {
			return "", fmt.Errorf("AWS credentials incomplete in secret (access-key-id and secret-access-key required)")
		}

		// Create AWS config with static credentials
		configOpts := []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				accessKeyID,
				secretAccessKey,
				"",
			)),
		}

		// Add region if specified
		if region != "" {
			configOpts = append(configOpts, config.WithRegion(region))
		}

		cfg, err = config.LoadDefaultConfig(ctx, configOpts...)
		if err != nil {
			return "", fmt.Errorf("failed to load AWS config: %w", err)
		}
	}

	// Create ACM client
	acmClient := acm.NewFromConfig(cfg)

	// Import certificate
	input := &acm.ImportCertificateInput{
		Certificate: tlsCert,
		PrivateKey:  tlsKey,
		Tags: []acmtypes.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("certificate-operator"),
			},
			{
				Key:   aws.String("Domain"),
				Value: aws.String(cert.Spec.Domain),
			},
		},
	}

	result, err := acmClient.ImportCertificate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to import certificate to AWS ACM: %w", err)
	}

	// Return the ARN of the imported certificate
	return aws.ToString(result.CertificateArn), nil
}

// finalizeCertificate performs cleanup when Certificate is being deleted
func (r *CertificateReconciler) finalizeCertificate(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	log := logf.FromContext(ctx)
	log.Info("Finalizing Certificate", "name", cert.Name)

	// Cleanup AWS ACM certificate if it was uploaded
	if cert.Status.AWSCertificateARN != "" {
		if err := r.deleteFromAWS(ctx, cert); err != nil {
			log.Error(err, "Failed to delete certificate from AWS ACM", "arn", cert.Status.AWSCertificateARN)
			// Continue with other cleanup even if AWS deletion fails
		} else {
			log.Info("Successfully deleted certificate from AWS ACM", "arn", cert.Status.AWSCertificateARN)
		}
	}

	// Cleanup Cloudflare certificate if it was uploaded
	if cert.Status.CloudflareCertificateID != "" {
		if err := r.deleteFromCloudflare(ctx, cert); err != nil {
			log.Error(err, "Failed to delete certificate from Cloudflare", "id", cert.Status.CloudflareCertificateID)
			// Continue even if Cloudflare deletion fails
		} else {
			log.Info("Successfully deleted certificate from Cloudflare", "id", cert.Status.CloudflareCertificateID)
		}
	}

	// Note: Issuer and cert-manager Certificate will be automatically deleted via owner references
	log.Info("Certificate finalization complete")
	return nil
}

func (r *CertificateReconciler) deleteFromAWS(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	var cfg aws.Config
	var err error

	// Use the same credential logic as upload
	if cert.Spec.AWSSecretRef == "" {
		cfg, err = config.LoadDefaultConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to load AWS default config: %w", err)
		}
	} else {
		awsSecret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      cert.Spec.AWSSecretRef,
			Namespace: cert.Namespace,
		}, awsSecret); err != nil {
			return fmt.Errorf("failed to get AWS secret: %w", err)
		}

		accessKeyID := string(awsSecret.Data["access-key-id"])
		secretAccessKey := string(awsSecret.Data["secret-access-key"])
		region := string(awsSecret.Data["region"])

		if accessKeyID == "" || secretAccessKey == "" {
			return fmt.Errorf("AWS credentials incomplete in secret")
		}

		configOpts := []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				accessKeyID,
				secretAccessKey,
				"",
			)),
		}

		if region != "" {
			configOpts = append(configOpts, config.WithRegion(region))
		}

		cfg, err = config.LoadDefaultConfig(ctx, configOpts...)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}
	}

	acmClient := acm.NewFromConfig(cfg)

	// Delete the certificate
	_, err = acmClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
		CertificateArn: aws.String(cert.Status.AWSCertificateARN),
	})
	if err != nil {
		return fmt.Errorf("failed to delete certificate from AWS ACM: %w", err)
	}

	return nil
}

func (r *CertificateReconciler) deleteFromCloudflare(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	// Get Cloudflare credentials
	cfSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cert.Spec.CloudflareSecretRef,
		Namespace: cert.Namespace,
	}, cfSecret); err != nil {
		return fmt.Errorf("failed to get Cloudflare secret: %w", err)
	}

	apiToken := string(cfSecret.Data["api-token"])
	if apiToken == "" {
		return fmt.Errorf("api-token not found in Cloudflare secret")
	}

	if cert.Spec.CloudflareZoneID == "" {
		return fmt.Errorf("cloudflareZoneID not set, cannot delete certificate")
	}

	// Create Cloudflare client
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Delete certificate from Cloudflare using zone ID
	err = api.DeleteSSL(ctx, cert.Spec.CloudflareZoneID, cert.Status.CloudflareCertificateID)
	if err != nil {
		return fmt.Errorf("failed to delete certificate from Cloudflare: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certificatev1alpha1.Certificate{}).
		Owns(&certmanagerv1.Issuer{}).
		Owns(&certmanagerv1.Certificate{}).
		Named("certificate").
		Complete(r)
}
