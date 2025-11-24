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

package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certificatev1alpha1 "github.com/tae2089/certificate-operator/api/v1alpha1"
	awsdriver "github.com/tae2089/certificate-operator/internal/driver/aws"
	cloudflaredriver "github.com/tae2089/certificate-operator/internal/driver/cloudflare"
	kubernetesdriver "github.com/tae2089/certificate-operator/internal/driver/kubernetes"
	"github.com/tae2089/certificate-operator/internal/driver/types"
)

// CertificateManager orchestrates certificate operations across multiple drivers
type CertificateManager struct {
	certManager types.CertManager
	k8sClient   client.Client
	scheme      *runtime.Scheme
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(k8sClient client.Client, scheme *runtime.Scheme) *CertificateManager {
	return &CertificateManager{
		certManager: kubernetesdriver.NewDriver(k8sClient, scheme),
		k8sClient:   k8sClient,
		scheme:      scheme,
	}
}

// ProcessCertificate processes a certificate CR
func (m *CertificateManager) ProcessCertificate(ctx context.Context, cert *certificatev1alpha1.Certificate) (ctrl.Result, bool, error) {
	log := logf.FromContext(ctx)

	// Set default ClusterIssuer name if not specified
	clusterIssuerName := cert.Spec.ClusterIssuerName
	if clusterIssuerName == "" {
		clusterIssuerName = "letsencrypt-prod"
	}

	// Ensure cert-manager Certificate with ClusterIssuer reference
	certResult, err := m.certManager.EnsureCertificate(ctx, types.CertSpec{
		Name:              cert.Name + "-cert",
		Namespace:         cert.Namespace,
		Domain:            cert.Spec.Domain,
		ClusterIssuerName: clusterIssuerName,
		SecretName:        cert.Name + "-tls",
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(cert, certificatev1alpha1.GroupVersion.WithKind("Certificate")),
		},
	})
	if err != nil {
		return ctrl.Result{}, false, err
	}

	// Update status if needed
	statusUpdated := false
	if cert.Status.CertificateRef != certResult.Name {
		cert.Status.CertificateRef = certResult.Name
		statusUpdated = true
	}

	// Get TLS Secret
	tlsSecret, err := m.certManager.GetTLSSecret(ctx, cert.Name+"-tls", cert.Namespace)
	if err != nil {
		// Secret doesn't exist, wait for readiness
		result, waitErr := m.certManager.WaitForReadiness(ctx, certResult.Name, cert.Namespace)
		return result, statusUpdated, waitErr
	}

	if tlsSecret == nil {
		// Secret exists but is empty
		log.Info("TLS secret is empty, waiting...")
		return ctrl.Result{}, statusUpdated, nil
	}

	log.V(1).Info("TLS Secret found, proceeding with certificate upload")

	// Upload certificates to cloud providers if changed
	certChanged := m.uploadToCloudProviders(ctx, cert, tlsSecret.Certificate, tlsSecret.PrivateKey, &statusUpdated)

	// Update hash and timestamp if certificate was uploaded
	if certChanged && (cert.Status.CloudflareUploaded || cert.Status.AWSUploaded) {
		now := metav1.Now()
		cert.Status.LastUploadedCertHash = calculateCertHash(tlsSecret.Certificate)
		cert.Status.LastUploadedTime = &now
		statusUpdated = true
	}

	return ctrl.Result{}, statusUpdated, nil
}

// uploadToCloudProviders uploads certificates to configured cloud providers
func (m *CertificateManager) uploadToCloudProviders(
	ctx context.Context,
	cert *certificatev1alpha1.Certificate,
	tlsCert, tlsKey []byte,
	statusUpdated *bool,
) bool {
	log := logf.FromContext(ctx)

	// Calculate certificate hash to detect renewals
	currentCertHash := calculateCertHash(tlsCert)
	certChanged := currentCertHash != cert.Status.LastUploadedCertHash

	if certChanged {
		if cert.Status.LastUploadedCertHash != "" {
			log.Info("Certificate hash changed, re-uploading to cloud providers",
				"oldHash", cert.Status.LastUploadedCertHash,
				"newHash", currentCertHash)
		} else {
			log.Info("Certificate ready for initial upload", "hash", currentCertHash)
		}
	}

	certData := types.CertificateData{
		Domain:      cert.Spec.Domain,
		Certificate: tlsCert,
		PrivateKey:  tlsKey,
	}

	// Upload to Cloudflare if configured
	cloudflareEnabled := cert.Spec.CloudflareEnabled == nil || *cert.Spec.CloudflareEnabled
	if cert.Spec.CloudflareSecretRef != "" && cloudflareEnabled && certChanged {
		certData.ExistingID = cert.Status.CloudflareCertificateID
		driver := cloudflaredriver.NewDriver(cloudflaredriver.Config{
			Client:    m.k8sClient,
			SecretRef: cert.Spec.CloudflareSecretRef,
			Namespace: cert.Namespace,
			ZoneID:    cert.Spec.CloudflareZoneID,
		})

		result, err := driver.Upload(ctx, certData)
		if err != nil {
			log.Error(err, "Failed to upload to Cloudflare")
		} else {
			cert.Status.CloudflareUploaded = true
			cert.Status.CloudflareCertificateID = result.Identifier
			*statusUpdated = true
			log.Info("Successfully uploaded certificate to Cloudflare", "id", result.Identifier)
		}
	}

	// Upload to AWS ACM if configured
	if cert.Spec.AWS != nil && certChanged {
		certData.ExistingID = cert.Status.AWSCertificateARN
		driver := awsdriver.NewDriver(awsdriver.Config{
			Client:         m.k8sClient,
			CredentialType: cert.Spec.AWS.CredentialType,
			SecretRef:      cert.Spec.AWS.SecretRef,
			Namespace:      cert.Namespace,
			Domain:         cert.Spec.Domain,
		})

		result, err := driver.Upload(ctx, certData)
		if err != nil {
			log.Error(err, "Failed to upload to AWS")
		} else {
			cert.Status.AWSUploaded = true
			cert.Status.AWSCertificateARN = result.Identifier
			*statusUpdated = true
			log.Info("Successfully uploaded certificate to AWS ACM", "arn", result.Identifier)
		}
	}

	return certChanged
}

// Finalize performs cleanup when Certificate is being deleted
func (m *CertificateManager) Finalize(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	log := logf.FromContext(ctx)
	log.Info("Finalizing Certificate", "name", cert.Name)

	// Cleanup AWS ACM certificate if it was uploaded
	if cert.Status.AWSCertificateARN != "" {
		driver := awsdriver.NewDriver(awsdriver.Config{
			Client:         m.k8sClient,
			CredentialType: cert.Spec.AWS.CredentialType,
			SecretRef:      cert.Spec.AWS.SecretRef,
			Namespace:      cert.Namespace,
			Domain:         cert.Spec.Domain,
		})

		if err := driver.Delete(ctx, cert.Status.AWSCertificateARN); err != nil {
			log.Error(err, "Failed to delete certificate from AWS ACM", "arn", cert.Status.AWSCertificateARN)
			// Continue with other cleanup even if AWS deletion fails
		} else {
			log.Info("Successfully deleted certificate from AWS ACM", "arn", cert.Status.AWSCertificateARN)
		}
	}

	// Cleanup Cloudflare certificate if it was uploaded
	if cert.Status.CloudflareCertificateID != "" {
		driver := cloudflaredriver.NewDriver(cloudflaredriver.Config{
			Client:    m.k8sClient,
			SecretRef: cert.Spec.CloudflareSecretRef,
			Namespace: cert.Namespace,
			ZoneID:    cert.Spec.CloudflareZoneID,
		})

		if err := driver.Delete(ctx, cert.Status.CloudflareCertificateID); err != nil {
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

// calculateCertHash calculates SHA256 hash of the certificate
func calculateCertHash(cert []byte) string {
	hash := sha256.Sum256(cert)
	return hex.EncodeToString(hash[:])
}
