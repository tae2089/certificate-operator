package router

import (
	"github.com/gin-gonic/gin"
	"github.com/tae2089/certificate-operator/internal/api/handler"
	"sigs.k8s.io/controller-runtime/pkg/client"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRouter creates and configures the Gin router
func SetupRouter(k8sClient client.Client) *gin.Engine {
	// Set Gin to release mode for production
	// gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// Health check endpoint
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	// Swagger documentation endpoint
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Create handlers
	certHandler := handler.NewCertificateHandler(k8sClient)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Certificate routes
		certificates := v1.Group("/certificates")
		{
			certificates.POST("", certHandler.CreateCertificate)
			certificates.GET("", certHandler.ListCertificates)
		}

		// Namespaced certificate routes
		namespaces := v1.Group("/namespaces")
		{
			namespaceCerts := namespaces.Group("/:namespace/certificates")
			{
				namespaceCerts.GET("", certHandler.ListCertificatesInNamespace)
				namespaceCerts.GET("/:name", certHandler.GetCertificate)
				namespaceCerts.PUT("/:name", certHandler.UpdateCertificate)
				namespaceCerts.DELETE("/:name", certHandler.DeleteCertificate)
			}
		}
	}

	return router
}
