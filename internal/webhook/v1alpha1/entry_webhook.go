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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	klapv1alpha1 "github.com/ripolin/klap/api/v1alpha1"
	"github.com/ripolin/klap/internal/controller"

	"github.com/go-ldap/ldap/v3"
)

// nolint:unused
// log is for logging in this package.
var entrylog = logf.Log.WithName("entry-resource")

// SetupEntryWebhookWithManager registers the webhook for Entry in the manager.
func SetupEntryWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&klapv1alpha1.Entry{}).
		WithValidator(&EntryCustomValidator{}).
		WithDefaulter(&EntryCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-klap-ripolin-github-com-v1alpha1-entry,mutating=true,failurePolicy=fail,sideEffects=None,groups=klap.ripolin.github.com,resources=entries,verbs=create;update,versions=v1alpha1,name=mentry-v1alpha1.kb.io,admissionReviewVersions=v1

// EntryCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Entry when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type EntryCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &EntryCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Entry.
func (d *EntryCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	entry, ok := obj.(*klapv1alpha1.Entry)

	if !ok {
		return fmt.Errorf("expected an Entry object but got %T", obj)
	}
	entrylog.Info("Defaulting for Entry", "name", entry.GetName())

	if entry.DeletionTimestamp == nil && !controllerutil.ContainsFinalizer(entry, controller.Finalizer) {
		controllerutil.AddFinalizer(entry, controller.Finalizer)
	}

	if entry.Spec.ServerRef.Namespace == nil {
		entry.Spec.ServerRef.Namespace = &entry.Namespace
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-klap-ripolin-github-com-v1alpha1-entry,mutating=false,failurePolicy=fail,sideEffects=None,groups=klap.ripolin.github.com,resources=entries,verbs=create;update,versions=v1alpha1,name=ventry-v1alpha1.kb.io,admissionReviewVersions=v1

// EntryCustomValidator struct is responsible for validating the Entry resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type EntryCustomValidator struct{}

var _ webhook.CustomValidator = &EntryCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Entry.
func (v *EntryCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	entry, ok := obj.(*klapv1alpha1.Entry)
	if !ok {
		return nil, fmt.Errorf("expected a Entry object but got %T", obj)
	}
	entrylog.Info("Validation for Entry upon creation", "name", entry.GetName())

	return nil, validateEntry(entry)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Entry.
func (v *EntryCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	entry, ok := newObj.(*klapv1alpha1.Entry)
	if !ok {
		return nil, fmt.Errorf("expected a Entry object for the newObj but got %T", newObj)
	}
	entrylog.Info("Validation for Entry upon update", "name", entry.GetName())

	return nil, validateEntry(entry)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Entry.
func (v *EntryCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	entry, ok := obj.(*klapv1alpha1.Entry)
	if !ok {
		return nil, fmt.Errorf("expected a Entry object but got %T", obj)
	}
	entrylog.Info("Validation for Entry upon deletion", "name", entry.GetName())

	return nil, nil
}

// validateEntry performs validation on the Entry resource.
func validateEntry(entry *klapv1alpha1.Entry) error {
	var allErrs field.ErrorList

	if _, err := ldap.ParseDN(*entry.Spec.DN); err != nil {
		fieldErr := field.Invalid(field.NewPath("spec").Child("dn"), entry.Name, "must be a valid distinguished name")
		allErrs = append(allErrs, fieldErr)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "klap.ripolin.github.com", Kind: "Entry"},
		entry.Name, allErrs)
}
