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

package cloudflare

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	drivertypes "github.com/tae2089/certificate-operator/internal/driver/types"
)

// Driver implements the CloudProvider interface for Cloudflare
type Driver struct {
	client    client.Client
	secretRef string
	namespace string
	zoneID    string
}

// Config holds Cloudflare driver configuration
type Config struct {
	Client    client.Client
	SecretRef string
	Namespace string
	ZoneID    string
}

// NewDriver creates a new Cloudflare driver
func NewDriver(cfg Config) *Driver {
	return &Driver{
		client:    cfg.Client,
		secretRef: cfg.SecretRef,
		namespace: cfg.Namespace,
		zoneID:    cfg.ZoneID,
	}
}

// Name returns the provider name
func (d *Driver) Name() string {
	return "cloudflare"
}

// Upload uploads a certificate to Cloudflare
func (d *Driver) Upload(ctx context.Context, certData drivertypes.CertificateData) (drivertypes.UploadResult, error) {
	log := logf.FromContext(ctx)

	api, err := d.getCloudflareClient(ctx)
	if err != nil {
		return drivertypes.UploadResult{}, err
	}

	// Delete old certificate if it exists (for renewal)
	if certData.ExistingID != "" {
		log.Info("Deleting old certificate from Cloudflare before upload", "id", certData.ExistingID)
		if err := api.DeleteSSL(ctx, d.zoneID, certData.ExistingID); err != nil {
			log.Error(err, "Failed to delete old certificate from Cloudflare, continuing with upload", "id", certData.ExistingID)
			// Continue with upload even if deletion fails
		}
	}

	// Upload custom SSL certificate to Cloudflare using zone ID
	sslCert, err := api.CreateSSL(ctx, d.zoneID, cloudflare.ZoneCustomSSLOptions{
		Certificate: string(certData.Certificate),
		PrivateKey:  string(certData.PrivateKey),
	})
	if err != nil {
		return drivertypes.UploadResult{}, fmt.Errorf("failed to upload certificate to Cloudflare: %w", err)
	}

	return drivertypes.UploadResult{
		Identifier: sslCert.ID,
	}, nil
}

// Delete deletes a certificate from Cloudflare
func (d *Driver) Delete(ctx context.Context, identifier string) error {
	api, err := d.getCloudflareClient(ctx)
	if err != nil {
		return err
	}

	// Delete certificate from Cloudflare using zone ID
	err = api.DeleteSSL(ctx, d.zoneID, identifier)
	if err != nil {
		return fmt.Errorf("failed to delete certificate from Cloudflare: %w", err)
	}

	return nil
}

// getCloudflareClient creates a Cloudflare API client
func (d *Driver) getCloudflareClient(ctx context.Context) (*cloudflare.API, error) {
	// Get Cloudflare credentials
	cfSecret := &corev1.Secret{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Name:      d.secretRef,
		Namespace: d.namespace,
	}, cfSecret); err != nil {
		return nil, fmt.Errorf("failed to get Cloudflare secret: %w", err)
	}

	apiToken := string(cfSecret.Data["api-token"])
	if apiToken == "" {
		return nil, fmt.Errorf("api-token not found in Cloudflare secret")
	}

	// Create Cloudflare client
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return api, nil
}
