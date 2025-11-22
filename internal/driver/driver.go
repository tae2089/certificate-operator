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
	"github.com/tae2089/certificate-operator/internal/driver/types"
)

// Re-export types for convenience
type (
	CloudProvider   = types.CloudProvider
	CertManager     = types.CertManager
	CertificateData = types.CertificateData
	UploadResult    = types.UploadResult
	CertSpec        = types.CertSpec
	CertResult      = types.CertResult
	TLSSecret       = types.TLSSecret
)
