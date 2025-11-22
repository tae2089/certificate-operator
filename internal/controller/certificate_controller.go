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

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certificatev1alpha1 "github.com/tae2089/certificate-operator/api/v1alpha1"
	"github.com/tae2089/certificate-operator/internal/driver"
)

const (
	certificateFinalizer = "certificate.println.kr/finalizer"
)

// CertificateReconciler reconciles a Certificate object
type CertificateReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Manager *driver.CertificateManager
}

// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certificate.println.kr,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cert certificatev1alpha1.Certificate
	if err := r.Get(ctx, req.NamespacedName, &cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !cert.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &cert)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(&cert, certificateFinalizer) {
		controllerutil.AddFinalizer(&cert, certificateFinalizer)
		if err := r.Update(ctx, &cert); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Process certificate using the manager
	result, statusUpdated, err := r.Manager.ProcessCertificate(ctx, &cert)
	if err != nil {
		log.Error(err, "Failed to process certificate")
		return ctrl.Result{}, err
	}

	// Update status if changed
	if statusUpdated {
		if err := r.Status().Update(ctx, &cert); err != nil {
			log.Error(err, "Failed to update Certificate status")
			return ctrl.Result{}, err
		}
	}

	// Return result from manager (may include requeue)
	return result, nil
}

// handleDeletion handles the deletion of a Certificate CR
func (r *CertificateReconciler) handleDeletion(ctx context.Context, cert *certificatev1alpha1.Certificate) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(cert, certificateFinalizer) {
		if err := r.Manager.Finalize(ctx, cert); err != nil {
			log.Error(err, "Failed to finalize Certificate")
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(cert, certificateFinalizer)
		if err := r.Update(ctx, cert); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// findCertificateForSecret maps a Secret to its owning Certificate CR.
// The Secret name follows the pattern "{certificate-name}-tls".
func (r *CertificateReconciler) findCertificateForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	// Only process secrets that end with "-tls"
	secretName := secret.GetName()
	if !strings.HasSuffix(secretName, "-tls") {
		return nil
	}

	// Extract certificate name by removing "-tls" suffix
	certName := strings.TrimSuffix(secretName, "-tls")

	log := logf.FromContext(ctx)
	log.V(1).Info("Secret changed, triggering reconcile for Certificate",
		"secret", secretName,
		"certificate", certName,
		"namespace", secret.GetNamespace())

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      certName,
				Namespace: secret.GetNamespace(),
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the certificate manager if not already set
	if r.Manager == nil {
		r.Manager = driver.NewCertificateManager(r.Client, r.Scheme)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&certificatev1alpha1.Certificate{}).
		Owns(&certmanagerv1.Issuer{}).
		Owns(&certmanagerv1.Certificate{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findCertificateForSecret),
		).
		Named("certificate").
		Complete(r)
}
