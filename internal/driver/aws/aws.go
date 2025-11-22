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

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	drivertypes "github.com/tae2089/certificate-operator/internal/driver/types"
)

// Driver implements the CloudProvider interface for AWS ACM
type Driver struct {
	client    client.Client
	secretRef string
	namespace string
	domain    string
}

// Config holds AWS driver configuration
type Config struct {
	Client    client.Client
	SecretRef string // Empty string means use IRSA/Instance Profile
	Namespace string
	Domain    string
}

// NewDriver creates a new AWS ACM driver
func NewDriver(cfg Config) *Driver {
	return &Driver{
		client:    cfg.Client,
		secretRef: cfg.SecretRef,
		namespace: cfg.Namespace,
		domain:    cfg.Domain,
	}
}

// Name returns the provider name
func (d *Driver) Name() string {
	return "aws"
}

// Upload uploads a certificate to AWS ACM
func (d *Driver) Upload(ctx context.Context, certData drivertypes.CertificateData) (drivertypes.UploadResult, error) {
	log := logf.FromContext(ctx)

	cfg, err := d.loadAWSConfig(ctx)
	if err != nil {
		return drivertypes.UploadResult{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create ACM client
	acmClient := acm.NewFromConfig(cfg)

	// Import certificate (re-import if ARN exists for renewal)
	input := &acm.ImportCertificateInput{
		Certificate: certData.Certificate,
		PrivateKey:  certData.PrivateKey,
		Tags: []acmtypes.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("certificate-operator"),
			},
			{
				Key:   aws.String("Domain"),
				Value: aws.String(certData.Domain),
			},
		},
	}

	// If certificate already exists, re-import using the same ARN
	if certData.ExistingID != "" {
		log.Info("Re-importing certificate to existing ARN", "arn", certData.ExistingID)
		input.CertificateArn = aws.String(certData.ExistingID)
	}

	result, err := acmClient.ImportCertificate(ctx, input)
	if err != nil {
		return drivertypes.UploadResult{}, fmt.Errorf("failed to import certificate to AWS ACM: %w", err)
	}

	return drivertypes.UploadResult{
		Identifier: aws.ToString(result.CertificateArn),
	}, nil
}

// Delete deletes a certificate from AWS ACM
func (d *Driver) Delete(ctx context.Context, identifier string) error {
	cfg, err := d.loadAWSConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	acmClient := acm.NewFromConfig(cfg)

	// Delete the certificate
	_, err = acmClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
		CertificateArn: aws.String(identifier),
	})
	if err != nil {
		return fmt.Errorf("failed to delete certificate from AWS ACM: %w", err)
	}

	return nil
}

// loadAWSConfig loads AWS configuration from secret or default credential chain
func (d *Driver) loadAWSConfig(ctx context.Context) (aws.Config, error) {
	log := logf.FromContext(ctx)

	// If secretRef is empty, use default credential chain (IRSA, Instance Profile, etc.)
	if d.secretRef == "" {
		log.Info("Using AWS default credential chain (IRSA/Instance Profile)")
		return config.LoadDefaultConfig(ctx)
	}

	// Get AWS credentials from Secret
	awsSecret := &corev1.Secret{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Name:      d.secretRef,
		Namespace: d.namespace,
	}, awsSecret); err != nil {
		return aws.Config{}, fmt.Errorf("failed to get AWS secret: %w", err)
	}

	accessKeyID := string(awsSecret.Data["access-key-id"])
	secretAccessKey := string(awsSecret.Data["secret-access-key"])
	region := string(awsSecret.Data["region"])

	if accessKeyID == "" || secretAccessKey == "" {
		return aws.Config{}, fmt.Errorf("AWS credentials incomplete in secret (access-key-id and secret-access-key required)")
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

	return config.LoadDefaultConfig(ctx, configOpts...)
}
