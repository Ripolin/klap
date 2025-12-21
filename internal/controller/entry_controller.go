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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"

	klapv1alpha1 "github.com/ripolin/klap/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-ldap/ldap/v3"
)

const (
	BaseDN              = "base_dn"
	BindDN              = "bind_dn"
	CACertName          = "ca.crt"
	Password            = "password"
	StartTLS            = "start_tls"
	Url                 = "url"
	requeueAfterSuccess = 5 * time.Minute
	requeueAfterError   = time.Minute
	typeAvailable       = "Available"
)

var Finalizer = fmt.Sprintf("%s/finalizer", klapv1alpha1.GroupVersion.Group)

// EntryReconciler reconciles a Entry object
type EntryReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=klap.ripolin.github.com,resources=entries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=klap.ripolin.github.com,resources=entries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=klap.ripolin.github.com,resources=entries/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *EntryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		cli    ldap.Client
		entry  klapv1alpha1.Entry
		tlsCfg = &tls.Config{}
		log    = logf.FromContext(ctx)
	)

	if cacert, err := x509.SystemCertPool(); err != nil {
		log.Error(err, "Unable to load system cacerts")
		tlsCfg.RootCAs = x509.NewCertPool()
	} else {
		tlsCfg.RootCAs = cacert
	}

	if err := r.Get(ctx, req.NamespacedName, &entry); err != nil {
		if apierrs.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if entry.DeletionTimestamp != nil && !entry.Spec.Prune && controllerutil.RemoveFinalizer(&entry, Finalizer) {
		if err := r.Update(ctx, &entry); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	serverConfig, err := r.getServerConfig(ctx, &entry)

	if err != nil {
		return r.setStatusUnavailable(ctx, &entry, err)
	}

	if entry.Spec.TlsSecretRef.Name != nil {
		tlsConfig, err := r.getTlsConfig(ctx, &entry)
		if err != nil {
			return r.setStatusUnavailable(ctx, &entry, err)
		}
		if _, ok := tlsConfig.Data[CACertName]; ok {
			tlsCfg.RootCAs.AppendCertsFromPEM(tlsConfig.Data[CACertName])
		}
	}

	serverUrl, err := url.Parse(string(serverConfig.Data[Url]))

	if err != nil {
		return r.setStatusUnavailable(ctx, &entry, err)
	}

	tlsCfg.ServerName = serverUrl.Hostname()

	cli, err = ldap.DialURL(serverUrl.String(), ldap.DialWithTLSConfig(tlsCfg))

	if err != nil {
		return r.setStatusUnavailable(ctx, &entry, err)
	}

	if raw, ok := serverConfig.Data[StartTLS]; ok && serverUrl.Scheme == "ldap" {
		if startTls, _ := strconv.ParseBool(string(raw)); startTls {
			if err = cli.StartTLS(tlsCfg); err != nil {
				return r.setStatusUnavailable(ctx, &entry, err)
			}
		}
	}

	err = cli.Bind(string(serverConfig.Data[BindDN]), string(serverConfig.Data[Password]))

	if err != nil {
		return r.setStatusUnavailable(ctx, &entry, err)
	}

	defer func() {
		err = cli.Unbind()

		if err != nil {
			log.Error(err, err.Error())
		}
	}()

	if entry.DeletionTimestamp != nil {

		err := cli.Del(ldap.NewDelRequest(*entry.Spec.DN, []ldap.Control{}))
		if err != nil {
			return r.setStatusUnavailable(ctx, &entry, err)
		}

		if controllerutil.RemoveFinalizer(&entry, Finalizer) {
			if err := r.Update(ctx, &entry); err != nil {
				return ctrl.Result{}, err
			}
		}

		log.V(1).Info("Entry deleted", "dn", entry.Spec.DN)

		return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil

	}

	baseDN := string(serverConfig.Data[BaseDN])

	if entry.Status.EntryUUID != nil {
		err = r.updateEntry(cli, &entry, &baseDN)
	} else {
		err = r.addEntry(cli, &entry, &baseDN)
	}

	if err != nil {
		return r.setStatusUnavailable(ctx, &entry, err)
	}

	return r.setStatusAvailable(ctx, &entry)
}

// getServerConfig retrieves the server configuration secret referenced by the Entry.
func (r *EntryReconciler) getServerConfig(ctx context.Context, entry *klapv1alpha1.Entry) (*corev1.Secret, error) {
	var (
		secret = &corev1.Secret{}
		ref    = &types.NamespacedName{
			Name:      *entry.Spec.ServerSecretRef.Name,
			Namespace: *entry.Spec.ServerSecretRef.Namespace,
		}
	)
	err := r.Get(ctx, *ref, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// getTlsConfig retrieves the TLS configuration secret referenced by the Entry.
func (r *EntryReconciler) getTlsConfig(ctx context.Context, entry *klapv1alpha1.Entry) (*corev1.Secret, error) {
	var (
		secret = &corev1.Secret{}
		ref    = &types.NamespacedName{
			Name:      *entry.Spec.TlsSecretRef.Name,
			Namespace: *entry.Spec.TlsSecretRef.Namespace,
		}
	)
	err := r.Get(ctx, *ref, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// addEntry adds a new LDAP entry based on the provided Entry specification.
func (r *EntryReconciler) addEntry(cli ldap.Client, entry *klapv1alpha1.Entry, baseDN *string) error {
	var (
		request = ldap.NewAddRequest(*entry.Spec.DN, []ldap.Control{})
		search  = &ldap.SearchRequest{
			BaseDN:     *baseDN,
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     fmt.Sprintf("(entryDN=%s)", *entry.Spec.DN),
			Attributes: []string{"entryUUID"},
		}
	)

	for k, v := range entry.Spec.Attributes {
		request.Attributes = append(request.Attributes, ldap.Attribute{
			Type: k,
			Vals: v,
		})
	}

	if err := cli.Add(request); err != nil {
		if !ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
			return err
		}
	}

	if searchResult, err := cli.Search(search); err != nil {
		return err
	} else {
		entryUUID := searchResult.Entries[0].GetAttributeValue("entryUUID")
		entry.Status.EntryUUID = &entryUUID
	}

	return nil
}

// updateEntry updates an existing LDAP entry based on the provided Entry specification.
func (r *EntryReconciler) updateEntry(cli ldap.Client, entry *klapv1alpha1.Entry, baseDN *string) error {
	var (
		request = ldap.NewModifyRequest(*entry.Spec.DN, []ldap.Control{})
		search  = &ldap.SearchRequest{
			BaseDN:     *baseDN,
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     fmt.Sprintf("(entryUUID=%s)", *entry.Status.EntryUUID),
			Attributes: []string{"*"},
		}
	)

	if searchResult, err := cli.Search(search); err != nil {
		return err
	} else {

		if len(searchResult.Entries) == 0 {
			entryUUID := *entry.Status.EntryUUID
			entry.Status.EntryUUID = nil
			return fmt.Errorf("entry %s not found", entryUUID)
		}

		current := searchResult.Entries[0]
		dn, _ := ldap.ParseDN(*entry.Spec.DN)

		if current.DN != *entry.Spec.DN {
			newSup := &ldap.DN{
				RDNs: []*ldap.RelativeDN{},
			}
			newSup.RDNs = append(newSup.RDNs, dn.RDNs[1:]...)
			moddn := ldap.NewModifyDNRequest(
				current.DN,
				dn.RDNs[0].String(),
				true,
				newSup.String(),
			)
			if err = cli.ModifyDN(moddn); err != nil {
				return err
			}
			r.Recorder.Eventf(entry, "Normal", "Success", "entry DN updated to %s", *entry.Spec.DN)
		}

		for k, v := range entry.Spec.Attributes {
			if len(current.GetAttributeValues(k)) == 0 {
				request.Add(k, v)
				continue
			}
			if entry.Spec.Force {
				if slices.Compare(v, current.GetAttributeValues(k)) != 0 {
					request.Replace(k, v)
				}
			} else {
				for _, val := range v {
					if !slices.Contains(current.GetAttributeValues(k), val) {
						request.Add(k, []string{val})
					}
				}
			}
		}

		if entry.Spec.Force {
			for _, attr := range current.Attributes {
				if attr.Name == dn.RDNs[0].Attributes[0].Type {
					continue
				}
				if _, ok := entry.Spec.Attributes[attr.Name]; !ok {
					request.Delete(attr.Name, []string{})
				}
			}
		}
	}

	if len(request.Changes) > 0 {
		return cli.Modify(request)
	}

	return nil
}

// setStatusAvailable updates the Entry status to Available.
func (r *EntryReconciler) setStatusAvailable(ctx context.Context, entry *klapv1alpha1.Entry) (ctrl.Result, error) {

	if meta.SetStatusCondition(&entry.Status.Conditions, metav1.Condition{
		Type:               typeAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             "Synchronized",
		Message:            "Entry is synchronized with the remote server",
		ObservedGeneration: entry.Generation,
	}) {
		r.Recorder.Eventf(entry, "Normal", "Success", "entry %s reconciled", *entry.Spec.DN)
		if err := r.Status().Update(ctx, entry); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil
}

// setStatusUnavailable updates the Entry status to Unavailable with the provided error message.
func (r *EntryReconciler) setStatusUnavailable(ctx context.Context, entry *klapv1alpha1.Entry, err error) (ctrl.Result, error) {

	r.Recorder.Event(entry, "Warning", "Error", err.Error())

	if meta.SetStatusCondition(&entry.Status.Conditions, metav1.Condition{
		Type:               typeAvailable,
		Status:             metav1.ConditionFalse,
		Reason:             "Unsynchronized",
		Message:            "Entry is not synchronized with the remote server",
		ObservedGeneration: entry.Generation,
	}) {
		if err := r.Status().Update(ctx, entry); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfterError}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EntryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&klapv1alpha1.Entry{}).
		Named("entry").
		Complete(r)
}
