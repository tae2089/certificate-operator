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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CertificateSpec defines the desired state of Certificate.
type CertificateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Domain is the domain name for the certificate.
	Domain string `json:"domain"`

	// Email is the email address for ACME registration.
	Email string `json:"email"`

	// IssuerName is the name of the Issuer to create or use.
	// +optional
	IssuerName string `json:"issuerName,omitempty"`

	// IngressClassName is the name of the Ingress Class to be used for the HTTP01 solver.
	// Defaults to "nginx" if not specified.
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`

	// CloudflareSecretRef is the name of the Secret containing Cloudflare credentials (api-token).
	// +optional
	CloudflareSecretRef string `json:"cloudflareSecretRef,omitempty"`

	// CloudflareZoneID is the Cloudflare Zone ID where the certificate will be uploaded.
	// Required if CloudflareSecretRef is set.
	// +optional
	CloudflareZoneID string `json:"cloudflareZoneID,omitempty"`

	// CloudflareEnabled controls whether to upload certificate to Cloudflare.
	// Defaults to true if CloudflareSecretRef is set.
	// +optional
	CloudflareEnabled *bool `json:"cloudflareEnabled,omitempty"`

	// AWSSecretRef is the name of the Secret containing AWS credentials (access-key-id, secret-access-key, region).
	// +optional
	AWSSecretRef string `json:"awsSecretRef,omitempty"`

	// AWSEnabled controls whether to upload certificate to AWS ACM.
	// Defaults to true if AWSSecretRef is set, or if using IAM Role.
	// +optional
	AWSEnabled *bool `json:"awsEnabled,omitempty"`
}

// CertificateStatus defines the observed state of Certificate.
type CertificateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// IssuerRef references the created Issuer.
	IssuerRef string `json:"issuerRef,omitempty"`

	// CertificateRef references the created Certificate.
	CertificateRef string `json:"certificateRef,omitempty"`

	// CloudflareUploaded is true if the certificate has been uploaded to Cloudflare.
	CloudflareUploaded bool `json:"cloudflareUploaded,omitempty"`

	// AWSUploaded is true if the certificate has been uploaded to AWS ACM.
	AWSUploaded bool `json:"awsUploaded,omitempty"`

	// AWSCertificateARN is the ARN of the certificate in AWS ACM.
	AWSCertificateARN string `json:"awsCertificateARN,omitempty"`

	// CloudflareCertificateID is the ID of the certificate in Cloudflare.
	CloudflareCertificateID string `json:"cloudflareCertificateID,omitempty"`

	// LastUploadedCertHash is the SHA256 hash of the last uploaded certificate.
	// Used to detect certificate renewals.
	// +optional
	LastUploadedCertHash string `json:"lastUploadedCertHash,omitempty"`

	// LastUploadedTime is the timestamp of the last successful upload to cloud providers.
	// +optional
	LastUploadedTime *metav1.Time `json:"lastUploadedTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Certificate is the Schema for the certificates API.
type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertificateSpec   `json:"spec,omitempty"`
	Status CertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertificateList contains a list of Certificate.
type CertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Certificate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Certificate{}, &CertificateList{})
}
