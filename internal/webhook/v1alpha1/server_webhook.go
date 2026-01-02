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
	"context"
	"fmt"
	"net/url"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	klapv1alpha1 "github.com/ripolin/klap/api/v1alpha1"

	"github.com/go-ldap/ldap/v3"
)

const (
	CACertName = "ca.crt"
	Password   = "password"
)

// nolint:unused
// log is for logging in this package.
var serverlog = logf.Log.WithName("server-resource")

// SetupServerWebhookWithManager registers the webhook for Server in the manager.
func SetupServerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&klapv1alpha1.Server{}).
		WithValidator(&ServerCustomValidator{}).
		WithDefaulter(&ServerCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-klap-ripolin-github-com-v1alpha1-server,mutating=true,failurePolicy=fail,sideEffects=None,groups=klap.ripolin.github.com,resources=servers,verbs=create;update,versions=v1alpha1,name=mserver-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Server when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ServerCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ServerCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Server.
func (d *ServerCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	server, ok := obj.(*klapv1alpha1.Server)

	if !ok {
		return fmt.Errorf("expected an Server object but got %T", obj)
	}
	serverlog.Info("Defaulting for Server", "name", server.GetName())

	if server.Spec.PasswordSecretRef.Namespace == nil {
		server.Spec.PasswordSecretRef.Namespace = &server.Namespace
	}

	if server.Spec.PasswordSecretRef.Key == nil {
		key := Password
		server.Spec.PasswordSecretRef.Key = &key
	}

	if server.Spec.TlsSecretRef.Name != nil {
		if server.Spec.TlsSecretRef.Namespace == nil {
			server.Spec.TlsSecretRef.Namespace = &server.Namespace
		}
		if server.Spec.TlsSecretRef.Key == nil {
			key := CACertName
			server.Spec.TlsSecretRef.Key = &key
		}
	}

	return nil
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-klap-ripolin-github-com-v1alpha1-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=klap.ripolin.github.com,resources=servers,verbs=create;update,versions=v1alpha1,name=vserver-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerCustomValidator struct is responsible for validating the Server resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServerCustomValidator struct{}

var _ webhook.CustomValidator = &ServerCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	server, ok := obj.(*klapv1alpha1.Server)
	if !ok {
		return nil, fmt.Errorf("expected a Server object but got %T", obj)
	}
	serverlog.Info("Validation for Server upon creation", "name", server.GetName())

	return nil, validateServer(server)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	server, ok := newObj.(*klapv1alpha1.Server)
	if !ok {
		return nil, fmt.Errorf("expected a Server object for the newObj but got %T", newObj)
	}
	serverlog.Info("Validation for Server upon update", "name", server.GetName())

	return nil, validateServer(server)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	server, ok := obj.(*klapv1alpha1.Server)
	if !ok {
		return nil, fmt.Errorf("expected a Server object but got %T", obj)
	}
	serverlog.Info("Validation for Server upon deletion", "name", server.GetName())

	return nil, nil
}

// validateServer performs the actual validation of the Server resource.
func validateServer(server *klapv1alpha1.Server) error {
	var allErrs field.ErrorList

	if _, err := ldap.ParseDN(*server.Spec.BaseDN); err != nil {
		fieldErr := field.Invalid(field.NewPath("spec").Child("baseDN"), server.Name, "must be a valid distinguished name")
		allErrs = append(allErrs, fieldErr)
	}

	if _, err := ldap.ParseDN(*server.Spec.BindDN); err != nil {
		fieldErr := field.Invalid(field.NewPath("spec").Child("bindDN"), server.Name, "must be a valid distinguished name")
		allErrs = append(allErrs, fieldErr)
	}

	if serverUrl, err := url.Parse(*server.Spec.Url); err != nil || !slices.Contains([]string{"ldap", "ldaps"}, serverUrl.Scheme) {
		fieldErr := field.Invalid(field.NewPath("spec").Child("url"), server.Spec.Url, "must be a valid LDAP URL")
		allErrs = append(allErrs, fieldErr)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "klap.ripolin.github.com", Kind: "Server"},
		server.Name, allErrs)
}
