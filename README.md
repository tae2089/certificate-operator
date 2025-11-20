# certificate-operator

Kubernetes operator that automates certificate management using cert-manager and uploads certificates to cloud providers (Cloudflare, AWS ACM).

## Features

- 🔐 Automatic certificate provisioning via cert-manager (ACME HTTP-01)
- ☁️ Auto-upload to Cloudflare and AWS ACM
- 🔄 Periodic readiness checks (1-minute interval)
- 🎯 Simple CRD-based configuration

## Prerequisites

- Kubernetes cluster
- [cert-manager](https://cert-manager.io/) installed
- Ingress controller (e.g., nginx-ingress)

## Installation

```bash
# Install CRDs
make install

# Deploy operator
make deploy
```

## Usage

### Basic Certificate

Create a Certificate resource to provision a TLS certificate:

```yaml
apiVersion: certificate.println.kr/v1alpha1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  domain: "example.com"
  email: "admin@example.com"
  ingressClassName: "nginx"  # Optional, defaults to "nginx"
```

### With Cloudflare Upload

1. Create a Secret with Cloudflare credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
stringData:
  api-token: "your-cloudflare-api-token"
```

**Required Secret Keys:**
- `api-token`: Cloudflare API token with SSL certificate permissions

**Note:** You also need to specify `cloudflareZoneID` in the Certificate spec.

2. Reference the Secret in your Certificate:

```yaml
apiVersion: certificate.println.kr/v1alpha1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  domain: "example.com"
  email: "admin@example.com"
  cloudflareSecretRef: "cloudflare-credentials"
  cloudflareZoneID: "your-zone-id-here"
```

### With AWS ACM Upload

#### Option 1: Using IAM Role (Recommended)

For production environments, use IRSA (IAM Role for Service Account) or EC2 Instance Profile:

1. Set up IAM Role with ACM permissions:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "acm:ImportCertificate",
        "acm:AddTagsToCertificate"
      ],
      "Resource": "*"
    }
  ]
}
```

2. Reference in Certificate (no Secret needed):
```yaml
apiVersion: certificate.println.kr/v1alpha1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  domain: "example.com"
  email: "admin@example.com"
  # awsSecretRef is omitted - will use IAM Role
```

**Note:** When `awsSecretRef` is omitted, the operator uses AWS default credential chain (IRSA → Instance Profile → Environment variables).

#### Option 2: Using Static Credentials

1. Create a Secret with AWS credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default
type: Opaque
stringData:
  access-key-id: "AKIA..."
  secret-access-key: "..."
  region: "us-east-1"  # Optional
```

**Required Secret Keys:**
- `access-key-id`: AWS Access Key ID (required)
- `secret-access-key`: AWS Secret Access Key (required)
- `region`: AWS region (optional - uses default chain if omitted)

2. Reference the Secret in your Certificate:

```yaml
apiVersion: certificate.println.kr/v1alpha1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  domain: "example.com"
  email: "admin@example.com"
  awsSecretRef: "aws-credentials"
```

### Complete Example (Both Providers)

```yaml
apiVersion: certificate.println.kr/v1alpha1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  domain: "example.com"
  email: "admin@example.com"
  ingressClassName: "nginx"
  cloudflareSecretRef: "cloudflare-credentials"
  awsSecretRef: "aws-credentials"
```

## How It Works

1. **Issuer Creation**: Operator creates a cert-manager Issuer (ACME staging by default)
2. **Certificate Request**: Creates a cert-manager Certificate resource
3. **Readiness Check**: Polls every minute until Issuer and Certificate are ready
4. **Secret Retrieval**: Fetches the generated TLS certificate from the Secret
5. **Cloud Upload**: Uploads to configured cloud providers
6. **Status Update**: Updates CR status with upload results

## CRD Specification

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | Yes | Domain name for the certificate |
| `email` | string | Yes | Email for ACME registration |
| `issuerName` | string | No | Custom Issuer name (defaults to `{cert-name}-issuer`) |
| `ingressClassName` | string | No | Ingress class for HTTP-01 solver (defaults to `nginx`) |
| `cloudflareSecretRef` | string | No | Secret name containing Cloudflare credentials |
| `cloudflareEnabled` | bool | No | Enable/disable Cloudflare upload (defaults to true if secret is set) |
| `awsSecretRef` | string | No | Secret name containing AWS credentials |
| `awsEnabled` | bool | No | Enable/disable AWS upload (defaults to true if secret is set) |

### Usage Examples

**Disable AWS upload temporarily:**
```yaml
spec:
  domain: "example.com"
  email: "admin@example.com"
  awsSecretRef: "aws-credentials"
  awsEnabled: false  # Temporarily disable
```

**Use only Cloudflare:**
```yaml
spec:
  domain: "example.com"
  email: "admin@example.com"
  cloudflareSecretRef: "cloudflare-credentials"
  # awsSecretRef omitted - AWS upload disabled
```

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `issuerRef` | string | Name of the created Issuer |
| `certificateRef` | string | Name of the created cert-manager Certificate |
| `cloudflareUploaded` | bool | True if uploaded to Cloudflare |
| `awsUploaded` | bool | True if uploaded to AWS ACM |

## Development

```bash
# Run tests
make test

# Build locally
make build

# Run locally (requires kubeconfig)
make run
```

## License

Apache License 2.0
