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

// Global constants
const (
	requeueAfterSuccess = 5 * time.Minute
	requeueAfterError   = time.Minute
	typeAvailable       = "Available"
)

// Active Directory constants
const (
	ActiveDirectory     = "activedirectory"
	ActiveDirectoryDN   = "distinguishedName"
	ActiveDirectoryGUID = "objectGUID"
)

// OpenLDAP constants
const (
	OpenLDAP     = "openldap"
	OpenLDAPDN   = "entryDN"
	OpenLDAPGUID = "entryUUID"
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
// +kubebuilder:rbac:groups=klap.ripolin.github.com,resources=servers,verbs=get;list;watch
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
		entry  = &klapv1alpha1.Entry{}
		tlsCfg = &tls.Config{}
		log    = logf.FromContext(ctx)
		opts   = []ldap.DialOpt{}
	)

	if cacert, err := x509.SystemCertPool(); err != nil {
		log.Error(err, "Unable to load system cacerts")
		tlsCfg.RootCAs = x509.NewCertPool()
	} else {
		tlsCfg.RootCAs = cacert
	}

	if err := r.Get(ctx, req.NamespacedName, entry); err != nil {
		if apierrs.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if entry.DeletionTimestamp != nil && !entry.Spec.Prune && controllerutil.RemoveFinalizer(entry, Finalizer) {
		if err := r.Update(ctx, entry); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	server, err := r.getServer(ctx, entry)

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
	}

	if server.Spec.TlsSecretRef.Name != nil {
		cacert, err := r.getCACert(ctx, server)
		if err != nil {
			return r.setStatusUnavailable(ctx, entry, err)
		}
		tlsCfg.RootCAs.AppendCertsFromPEM(cacert)
	}

	serverUrl, err := url.Parse(*server.Spec.Url)

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
	}

	tlsCfg.ServerName = serverUrl.Hostname()

	if serverUrl.Scheme == "ldaps" {
		opts = append(opts, ldap.DialWithTLSConfig(tlsCfg))
	}

	cli, err = ldap.DialURL(serverUrl.String(), opts...)

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
	}

	if *server.Spec.StartTLS && serverUrl.Scheme == "ldap" {
		if err = cli.StartTLS(tlsCfg); err != nil {
			return r.setStatusUnavailable(ctx, entry, err)
		}
	}

	password, err := r.getPassword(ctx, server)

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
	}

	err = cli.Bind(*server.Spec.BindDN, password)

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
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
			return r.setStatusUnavailable(ctx, entry, err)
		}

		if controllerutil.RemoveFinalizer(entry, Finalizer) {
			if err := r.Update(ctx, entry); err != nil {
				return ctrl.Result{}, err
			}
		}

		log.V(1).Info("Entry deleted", "dn", entry.Spec.DN)

		return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil

	}

	if entry.Status.GUID != nil {
		err = r.updateEntry(cli, entry, server)
	} else {
		err = r.addEntry(cli, entry, server)
	}

	if err != nil {
		return r.setStatusUnavailable(ctx, entry, err)
	}

	return r.setStatusAvailable(ctx, entry)
}

// getServer retrieves the server configuration secret referenced by the Entry.
func (r *EntryReconciler) getServer(ctx context.Context, entry *klapv1alpha1.Entry) (*klapv1alpha1.Server, error) {
	var (
		server = &klapv1alpha1.Server{}
		ref    = &types.NamespacedName{
			Name:      *entry.Spec.ServerRef.Name,
			Namespace: *entry.Spec.ServerRef.Namespace,
		}
	)
	err := r.Get(ctx, *ref, server)
	if err != nil {
		return nil, err
	}
	return server, nil
}

// getCACert retrieves the TLS certs bundle referenced by the Server.
func (r *EntryReconciler) getCACert(ctx context.Context, server *klapv1alpha1.Server) ([]byte, error) {
	var (
		secret = &corev1.Secret{}
		ref    = &types.NamespacedName{
			Name:      *server.Spec.TlsSecretRef.Name,
			Namespace: *server.Spec.TlsSecretRef.Namespace,
		}
	)
	err := r.Get(ctx, *ref, secret)
	if err != nil {
		return nil, err
	}
	if cacert, ok := secret.Data[*server.Spec.TlsSecretRef.Key]; ok {
		return cacert, nil
	}
	return nil, fmt.Errorf("%s key not found in secret %s", *server.Spec.TlsSecretRef.Key, ref.Name)
}

// getPassword retrieves the password secret referenced by the Server.
func (r *EntryReconciler) getPassword(ctx context.Context, server *klapv1alpha1.Server) (string, error) {
	var (
		secret = &corev1.Secret{}
		ref    = &types.NamespacedName{
			Name:      *server.Spec.PasswordSecretRef.Name,
			Namespace: *server.Spec.PasswordSecretRef.Namespace,
		}
	)
	err := r.Get(ctx, *ref, secret)
	if err != nil {
		return "", err
	}
	if password, ok := secret.Data[*server.Spec.PasswordSecretRef.Key]; ok {
		return string(password), nil
	}
	return "", fmt.Errorf("%s key not found in secret %s", *server.Spec.PasswordSecretRef.Key, ref.Name)
}

// addEntry adds a new LDAP entry based on the provided Entry specification.
func (r *EntryReconciler) addEntry(cli ldap.Client, entry *klapv1alpha1.Entry, server *klapv1alpha1.Server) error {
	var (
		add    = ldap.NewAddRequest(*entry.Spec.DN, []ldap.Control{})
		search = ldap.NewSearchRequest(
			*server.Spec.BaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(%s=%s)", OpenLDAPDN, *entry.Spec.DN),
			[]string{OpenLDAPGUID},
			nil,
		)
	)

	if *server.Spec.Implementation == ActiveDirectory {
		search.Filter = fmt.Sprintf("(%s=%s)", ActiveDirectoryDN, *entry.Spec.DN)
		search.Attributes = []string{ActiveDirectoryGUID}
	}

	for k, v := range entry.Spec.Attributes {
		add.Attributes = append(add.Attributes, ldap.Attribute{
			Type: k,
			Vals: v,
		})
	}

	if err := cli.Add(add); err != nil {
		if !ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
			return err
		}
	}

	if searchResult, err := cli.Search(search); err != nil {
		return err
	} else {
		guid := searchResult.Entries[0].GetAttributeValue(OpenLDAPGUID)
		if *server.Spec.Implementation == ActiveDirectory {
			guid = searchResult.Entries[0].GetAttributeValue(ActiveDirectoryGUID)
		}
		if guid == "" {
			return fmt.Errorf("unable to retrieve entry GUID")
		}
		entry.Status.GUID = &guid
	}

	return nil
}

// updateEntry updates an existing LDAP entry based on the provided Entry specification.
func (r *EntryReconciler) updateEntry(cli ldap.Client, entry *klapv1alpha1.Entry, server *klapv1alpha1.Server) error {
	var (
		modify = ldap.NewModifyRequest(*entry.Spec.DN, []ldap.Control{})
		search = ldap.NewSearchRequest(
			*server.Spec.BaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(%s=%s)", OpenLDAPGUID, *entry.Status.GUID),
			[]string{"*"},
			nil,
		)
	)

	if *server.Spec.Implementation == ActiveDirectory {
		search.Filter = fmt.Sprintf("(%s=%s)", ActiveDirectoryGUID, *entry.Status.GUID)
	}

	if searchResult, err := cli.Search(search); err != nil {
		return err
	} else {

		if len(searchResult.Entries) == 0 {
			guid := *entry.Status.GUID
			entry.Status.GUID = nil
			return fmt.Errorf("entry %s not found", guid)
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
				modify.Add(k, v)
				continue
			}
			if entry.Spec.Force {
				if slices.Compare(v, current.GetAttributeValues(k)) != 0 {
					modify.Replace(k, v)
				}
			} else {
				for _, val := range v {
					if !slices.Contains(current.GetAttributeValues(k), val) {
						modify.Add(k, []string{val})
					}
				}
			}
		}

		if entry.Spec.Force {
			for _, attr := range current.Attributes {

				// Skip the RDN attribute computed from the DN
				if attr.Name == dn.RDNs[0].Attributes[0].Type {
					continue
				}

				if _, ok := entry.Spec.Attributes[attr.Name]; !ok {
					modify.Delete(attr.Name, attr.Values)
				}
			}
		}
	}

	if len(modify.Changes) > 0 {
		return cli.Modify(modify)
	}

	return nil
}

// setStatusAvailable updates the Entry status to Available.
func (r *EntryReconciler) setStatusAvailable(ctx context.Context, entry *klapv1alpha1.Entry) (ctrl.Result, error) {

	if meta.SetStatusCondition(&entry.Status.Conditions, metav1.Condition{
		Type:               typeAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             "ReconciledSuccessfully",
		Message:            "Entry is reconciled successfully",
		ObservedGeneration: entry.Generation,
	}) {
		r.Recorder.Eventf(entry, "Normal", "Success", "entry %s is available", *entry.Spec.DN)
		if err := r.Status().Update(ctx, entry); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil
}

// setStatusUnavailable updates the Entry status to Unavailable with the provided error message.
func (r *EntryReconciler) setStatusUnavailable(ctx context.Context, entry *klapv1alpha1.Entry, err error) (ctrl.Result, error) {

	r.Recorder.Event(entry, "Warning", "Error", err.Error())

	status := metav1.ConditionFalse

	if entry.Status.GUID != nil {
		// If the entry has a GUID, it means it was previously created.
		// In this case, we set the status to Unknown to indicate that
		// the current state is uncertain due to the error.
		status = metav1.ConditionUnknown
	}

	if meta.SetStatusCondition(&entry.Status.Conditions, metav1.Condition{
		Type:               typeAvailable,
		Status:             status,
		Reason:             "ErrorOccurred",
		Message:            err.Error(),
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
