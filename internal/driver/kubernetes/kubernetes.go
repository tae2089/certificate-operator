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

package kubernetes

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	drivertypes "github.com/tae2089/certificate-operator/internal/driver/types"
)

// Driver implements the CertManager interface for Kubernetes cert-manager
type Driver struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewDriver creates a new Kubernetes cert-manager driver
func NewDriver(client client.Client, scheme *runtime.Scheme) *Driver {
	return &Driver{
		client: client,
		scheme: scheme,
	}
}

// EnsureCertificate creates or updates a cert-manager Certificate
func (d *Driver) EnsureCertificate(ctx context.Context, spec drivertypes.CertSpec) (*drivertypes.CertResult, error) {
	certReq := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, d.client, certReq, func() error {
		if certReq.Labels == nil {
			certReq.Labels = make(map[string]string)
		}
		certReq.Labels["app.kubernetes.io/managed-by"] = "certificate-operator"

		// Set owner references
		if len(spec.OwnerReferences) > 0 {
			certReq.OwnerReferences = spec.OwnerReferences
		}

		// Set default ClusterIssuer if not specified
		clusterIssuerName := spec.ClusterIssuerName
		if clusterIssuerName == "" {
			clusterIssuerName = "letsencrypt-prod"
		}

		certReq.Spec = certmanagerv1.CertificateSpec{
			DNSNames:   []string{spec.Domain},
			SecretName: spec.SecretName,
			IssuerRef: cmmeta.ObjectReference{
				Name:  clusterIssuerName,
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &drivertypes.CertResult{
		Certificate: certReq,
		Name:        certReq.Name,
	}, nil
}

// GetTLSSecret retrieves and validates a TLS Secret
func (d *Driver) GetTLSSecret(ctx context.Context, name, namespace string) (*drivertypes.TLSSecret, error) {
	secret := &corev1.Secret{}
	err := d.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, secret)

	if err != nil {
		return nil, err
	}

	tlsCert := secret.Data["tls.crt"]
	tlsKey := secret.Data["tls.key"]

	if len(tlsCert) == 0 || len(tlsKey) == 0 {
		return nil, nil // Empty secret, not ready yet
	}

	return &drivertypes.TLSSecret{
		Secret:      secret,
		Certificate: tlsCert,
		PrivateKey:  tlsKey,
	}, nil
}

// WaitForReadiness checks if Certificate is ready
func (d *Driver) WaitForReadiness(ctx context.Context, certName, namespace string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get Certificate
	cert := &certmanagerv1.Certificate{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Name:      certName,
		Namespace: namespace,
	}, cert); err != nil {
		return ctrl.Result{}, err
	}

	// Check if Certificate is Ready
	certReady := false
	for _, cond := range cert.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			certReady = true
			break
		}
	}

	if !certReady {
		log.Info("Waiting for Certificate to be ready", "certificate", certName)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Certificate is ready
	log.Info("Certificate is ready, waiting for TLS secret to be created", "certificate", certName)
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}
