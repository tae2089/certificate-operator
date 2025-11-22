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

package types

import (
	"context"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// CloudProvider manages certificate lifecycle in cloud providers
type CloudProvider interface {
	// Upload uploads a certificate to the cloud provider
	Upload(ctx context.Context, cert CertificateData) (UploadResult, error)

	// Delete deletes a certificate from the cloud provider
	Delete(ctx context.Context, identifier string) error

	// Name returns the provider name
	Name() string
}

// CertManager manages cert-manager resources in Kubernetes
type CertManager interface {
	// EnsureIssuer creates or updates a cert-manager Issuer
	EnsureIssuer(ctx context.Context, spec IssuerSpec) (*IssuerResult, error)

	// EnsureCertificate creates or updates a cert-manager Certificate
	EnsureCertificate(ctx context.Context, spec CertSpec) (*CertResult, error)

	// GetTLSSecret retrieves and validates a TLS Secret
	GetTLSSecret(ctx context.Context, name, namespace string) (*TLSSecret, error)

	// WaitForReadiness checks if Issuer and Certificate are ready
	WaitForReadiness(ctx context.Context, issuerName, certName, namespace string) (ctrl.Result, error)
}

// CertificateData holds certificate information for upload
type CertificateData struct {
	Domain      string
	Certificate []byte
	PrivateKey  []byte
	ExistingID  string // For renewals (ARN for AWS, ID for Cloudflare)
}

// UploadResult contains cloud provider upload results
type UploadResult struct {
	Identifier string // ARN for AWS, certificate ID for Cloudflare
}

// IssuerSpec contains specification for creating an Issuer
type IssuerSpec struct {
	Name             string
	Namespace        string
	Email            string
	IngressClassName string
	OwnerReferences  []metav1.OwnerReference
}

// IssuerResult contains the result of Issuer creation
type IssuerResult struct {
	Issuer *certmanagerv1.Issuer
	Name   string
}

// CertSpec contains specification for creating a Certificate
type CertSpec struct {
	Name            string
	Namespace       string
	Domain          string
	IssuerName      string
	SecretName      string
	OwnerReferences []metav1.OwnerReference
}

// CertResult contains the result of Certificate creation
type CertResult struct {
	Certificate *certmanagerv1.Certificate
	Name        string
}

// TLSSecret holds TLS certificate and key data
type TLSSecret struct {
	Secret      *corev1.Secret
	Certificate []byte
	PrivateKey  []byte
}
