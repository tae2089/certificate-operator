package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	certificatev1alpha1 "github.com/tae2089/certificate-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CertificateHandler handles HTTP requests for Certificate resources
type CertificateHandler struct {
	Client client.Client
}

// NewCertificateHandler creates a new CertificateHandler
func NewCertificateHandler(k8sClient client.Client) *CertificateHandler {
	return &CertificateHandler{
		Client: k8sClient,
	}
}

// CreateCertificateRequest represents the request body for creating a Certificate
type CreateCertificateRequest struct {
	Name      string                              `json:"name" binding:"required" example:"example-cert"`
	Namespace string                              `json:"namespace" binding:"required" example:"default"`
	Spec      certificatev1alpha1.CertificateSpec `json:"spec" binding:"required"`
}

// UpdateCertificateRequest represents the request body for updating a Certificate
type UpdateCertificateRequest struct {
	Spec certificatev1alpha1.CertificateSpec `json:"spec" binding:"required"`
}

// CertificateResponse represents a Certificate resource response
type CertificateResponse struct {
	Name      string                    `json:"name" example:"example-cert"`
	Namespace string                    `json:"namespace" example:"default"`
	Spec      CertificateSpecResponse   `json:"spec"`
	Status    CertificateStatusResponse `json:"status"`
}

// CertificateSpecResponse represents the spec of a Certificate
type CertificateSpecResponse struct {
	Domain string `json:"domain" example:"example.com"`
}

// CertificateStatusResponse represents the status of a Certificate
type CertificateStatusResponse struct {
	CertificateRef     string `json:"certificateRef,omitempty"`
	CloudflareUploaded bool   `json:"cloudflareUploaded"`
	AWSUploaded        bool   `json:"awsUploaded"`
	LastUploadedTime   string `json:"lastUploadedTime,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"resource not found"`
}

// convertToResponse converts a Certificate to CertificateResponse
func convertToResponse(cert *certificatev1alpha1.Certificate) CertificateResponse {
	var lastUploadedTime string
	if cert.Status.LastUploadedTime != nil {
		lastUploadedTime = cert.Status.LastUploadedTime.Format("2006-01-02T15:04:05Z07:00")
	}

	return CertificateResponse{
		Name:      cert.Name,
		Namespace: cert.Namespace,
		Spec: CertificateSpecResponse{
			Domain: cert.Spec.Domain,
		},
		Status: CertificateStatusResponse{
			CertificateRef:     cert.Status.CertificateRef,
			CloudflareUploaded: cert.Status.CloudflareUploaded,
			AWSUploaded:        cert.Status.AWSUploaded,
			LastUploadedTime:   lastUploadedTime,
		},
	}
}

// CreateCertificate godoc
// @Summary Create a new Certificate
// @Description Create a new Certificate resource in the specified namespace
// @Tags certificates
// @Accept json
// @Produce json
// @Param certificate body CreateCertificateRequest true "Certificate to create"
// @Success 201 {object} CertificateResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/certificates [post]
func (h *CertificateHandler) CreateCertificate(c *gin.Context) {
	var req CreateCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	cert := &certificatev1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: req.Spec,
	}

	if err := h.Client.Create(context.Background(), cert); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, convertToResponse(cert))
}

// ListCertificates godoc
// @Summary List all Certificates
// @Description Get a list of all Certificate resources across all namespaces
// @Tags certificates
// @Produce json
// @Success 200 {array} CertificateResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/certificates [get]
func (h *CertificateHandler) ListCertificates(c *gin.Context) {
	certList := &certificatev1alpha1.CertificateList{}
	if err := h.Client.List(context.Background(), certList); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	responses := make([]CertificateResponse, 0, len(certList.Items))
	for _, cert := range certList.Items {
		responses = append(responses, convertToResponse(&cert))
	}

	c.JSON(http.StatusOK, responses)
}

// ListCertificatesInNamespace godoc
// @Summary List Certificates in a namespace
// @Description Get a list of Certificate resources in a specific namespace
// @Tags certificates
// @Produce json
// @Param namespace path string true "Namespace"
// @Success 200 {array} CertificateResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/certificates [get]
func (h *CertificateHandler) ListCertificatesInNamespace(c *gin.Context) {
	namespace := c.Param("namespace")

	certList := &certificatev1alpha1.CertificateList{}
	if err := h.Client.List(context.Background(), certList, client.InNamespace(namespace)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	responses := make([]CertificateResponse, 0, len(certList.Items))
	for _, cert := range certList.Items {
		responses = append(responses, convertToResponse(&cert))
	}

	c.JSON(http.StatusOK, responses)
}

// GetCertificate godoc
// @Summary Get a Certificate
// @Description Get a specific Certificate resource by name and namespace
// @Tags certificates
// @Produce json
// @Param namespace path string true "Namespace"
// @Param name path string true "Certificate name"
// @Success 200 {object} CertificateResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/certificates/{name} [get]
func (h *CertificateHandler) GetCertificate(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	cert := &certificatev1alpha1.Certificate{}
	if err := h.Client.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, cert); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, convertToResponse(cert))
}

// UpdateCertificate godoc
// @Summary Update a Certificate
// @Description Update an existing Certificate resource
// @Tags certificates
// @Accept json
// @Produce json
// @Param namespace path string true "Namespace"
// @Param name path string true "Certificate name"
// @Param certificate body UpdateCertificateRequest true "Certificate updates"
// @Success 200 {object} CertificateResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/certificates/{name} [put]
func (h *CertificateHandler) UpdateCertificate(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	var req UpdateCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	cert := &certificatev1alpha1.Certificate{}
	if err := h.Client.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, cert); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	// Update spec with the provided spec
	cert.Spec = req.Spec
	if err := h.Client.Update(context.Background(), cert); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, convertToResponse(cert))
}

// DeleteCertificate godoc
// @Summary Delete a Certificate
// @Description Delete a Certificate resource
// @Tags certificates
// @Produce json
// @Param namespace path string true "Namespace"
// @Param name path string true "Certificate name"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/certificates/{name} [delete]
func (h *CertificateHandler) DeleteCertificate(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	cert := &certificatev1alpha1.Certificate{}
	if err := h.Client.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, cert); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.Client.Delete(context.Background(), cert); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
