package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appdeployv1 "github.com/ude-p/appdeploy/api/v1"
)

type AppDeployReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RESTMapper meta.RESTMapper
}

// +kubebuilder:rbac:groups=appdeploy.io,resources=appdeploys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=appdeploy.io,resources=appdeploys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=appdeploy.io,resources=appdeploys/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch;create;update;patch;delete

func (r *AppDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	var appdeploy appdeployv1.AppDeploy
	if err := r.Get(ctx, req.NamespacedName, &appdeploy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		if statusErr := r.updateStatus(ctx, &appdeploy, err); statusErr != nil && err == nil {
			err = statusErr
		}
	}()

	if validateErr := r.validate(&appdeploy); validateErr != nil {
		err = validateErr
		return ctrl.Result{}, err
	}

	if len(appdeploy.Spec.Secrets) > 0 {
		if esoErr := r.ensureESOConfigured(); esoErr != nil {
			err = esoErr
			return ctrl.Result{}, err
		}
	}

	targetNamespaces := appdeploy.Spec.Namespaces
	if len(appdeploy.Spec.SelectedNamespaces) > 0 {
		targetNamespaces = appdeploy.Spec.SelectedNamespaces
	}

	if namespaceErr := r.reconcileNamespaces(ctx, targetNamespaces); namespaceErr != nil {
		err = namespaceErr
		return ctrl.Result{}, err
	}

	for _, namespace := range targetNamespaces {
		if configMapErr := r.reconcileConfigMaps(ctx, namespace, appdeploy.Spec.ConfigMaps); configMapErr != nil {
			err = configMapErr
			return ctrl.Result{}, err
		}

		for i := range appdeploy.Spec.Secrets {
			secret := appdeploy.Spec.Secrets[i]
			if secret.Scope != "" && secret.Scope != namespace {
				continue
			}
			if secretErr := r.reconcileExternalSecret(ctx, namespace, &secret); secretErr != nil {
				err = secretErr
				return ctrl.Result{}, err
			}
		}

		for i := range appdeploy.Spec.Workloads {
			workload := appdeploy.Spec.Workloads[i]
			if workload.Scope != "" && workload.Scope != namespace {
				continue
			}
			switch workload.Kind {
			case "", "Deployment":
				if deploymentErr := r.reconcileDeployment(ctx, namespace, &workload); deploymentErr != nil {
					err = deploymentErr
					return ctrl.Result{}, err
				}
			case "StatefulSet":
				if statefulSetErr := r.reconcileStatefulSet(ctx, namespace, &workload); statefulSetErr != nil {
					err = statefulSetErr
					return ctrl.Result{}, err
				}
			case "Job":
				if jobErr := r.reconcileJob(ctx, namespace, &workload); jobErr != nil {
					err = jobErr
					return ctrl.Result{}, err
				}
			default:
				err = fmt.Errorf("spec.workloads[%d].kind %q is not supported", i, workload.Kind)
				return ctrl.Result{}, err
			}
			if workload.Kind != "Job" {
				if serviceErr := r.reconcileService(ctx, namespace, &workload); serviceErr != nil {
					err = serviceErr
					return ctrl.Result{}, err
				}
			}
		}

		for i := range appdeploy.Spec.Ingresses {
			ingress := appdeploy.Spec.Ingresses[i]
			if ingress.Scope != "" && ingress.Scope != namespace {
				continue
			}
			if ingressErr := r.reconcileIngress(ctx, namespace, &ingress); ingressErr != nil {
				err = ingressErr
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppDeployReconciler) validate(appdeploy *appdeployv1.AppDeploy) error {
	if len(appdeploy.Spec.Namespaces) == 0 {
		return fmt.Errorf("spec.namespaces must not be empty")
	}

	namespaceSet := make(map[string]struct{}, len(appdeploy.Spec.Namespaces))
	for i, input := range appdeploy.Spec.Namespaces {
		if _, ok := namespaceSet[input]; ok {
			return fmt.Errorf("spec.namespaces[%d] duplicates namespace %q", i, input)
		}
		namespaceSet[input] = struct{}{}
	}

	selectedNamespaceSet := make(map[string]struct{}, len(appdeploy.Spec.SelectedNamespaces))
	for i, selectedNamespace := range appdeploy.Spec.SelectedNamespaces {
		if _, ok := selectedNamespaceSet[selectedNamespace]; ok {
			return fmt.Errorf("spec.selectedNamespaces[%d] duplicates namespace %q", i, selectedNamespace)
		}
		selectedNamespaceSet[selectedNamespace] = struct{}{}
		if _, ok := namespaceSet[selectedNamespace]; !ok {
			return fmt.Errorf("spec.selectedNamespaces[%d] %q must match one of spec.namespaces", i, selectedNamespace)
		}
	}

	if err := validateConfigMaps(appdeploy.Spec.ConfigMaps, namespaceSet); err != nil {
		return err
	}

	for i, workload := range appdeploy.Spec.Workloads {
		if workload.Scope != "" {
			if _, ok := namespaceSet[workload.Scope]; !ok {
				return fmt.Errorf("spec.workloads[%d].scope %q must match one of spec.namespaces", i, workload.Scope)
			}
		}
		if workload.Kind != "Job" {
			if len(workload.ServicePorts) == 0 {
				return fmt.Errorf("spec.workloads[%d].servicePorts must be set when kind is %q", i, workload.Kind)
			}
			if len(workload.ContainerPorts) > 0 && len(workload.ContainerPorts) != len(workload.ServicePorts) {
				return fmt.Errorf("spec.workloads[%d].containerPorts must have the same length as servicePorts", i)
			}
			servicePorts := make(map[int32]struct{}, len(workload.ServicePorts))
			for j, servicePort := range workload.ServicePorts {
				if servicePort < 1 || servicePort > 65535 {
					return fmt.Errorf("spec.workloads[%d].servicePorts[%d] must be between 1 and 65535", i, j)
				}
				if _, ok := servicePorts[servicePort]; ok {
					return fmt.Errorf("spec.workloads[%d].servicePorts[%d] duplicates port %d", i, j, servicePort)
				}
				servicePorts[servicePort] = struct{}{}
			}
			for j, containerPort := range workload.ContainerPorts {
				if containerPort < 1 || containerPort > 65535 {
					return fmt.Errorf("spec.workloads[%d].containerPorts[%d] must be between 1 and 65535", i, j)
				}
			}
		}
		if workload.Kind == "StatefulSet" && workload.HeadlessServiceName == "" {
			return fmt.Errorf("spec.workloads[%d].headlessServiceName must be set when kind is StatefulSet", i)
		}
		for j, volumeMount := range workload.VolumeMounts {
			if volumeMount.ConfigMapName == "" && volumeMount.SecretName == "" {
				return fmt.Errorf("spec.workloads[%d].volumeMounts[%d] must set configMapName or secretName", i, j)
			}
			if volumeMount.ConfigMapName != "" && volumeMount.SecretName != "" {
				return fmt.Errorf("spec.workloads[%d].volumeMounts[%d] must not set both configMapName and secretName", i, j)
			}
		}
	}

	for i, secret := range appdeploy.Spec.Secrets {
		if secret.Scope != "" {
			if _, ok := namespaceSet[secret.Scope]; !ok {
				return fmt.Errorf("spec.secrets[%d].scope %q must match one of spec.namespaces", i, secret.Scope)
			}
		}
		switch secret.SecretStoreKind {
		case "SecretStore", "ClusterSecretStore":
		default:
			return fmt.Errorf("spec.secrets[%d].secretStoreKind %q is not supported", i, secret.SecretStoreKind)
		}
	}

	for i, ingress := range appdeploy.Spec.Ingresses {
		if ingress.Scope != "" {
			if _, ok := namespaceSet[ingress.Scope]; !ok {
				return fmt.Errorf("spec.ingresses[%d].scope %q must match one of spec.namespaces", i, ingress.Scope)
			}
		}
	}
	return nil
}

func (r *AppDeployReconciler) updateStatus(ctx context.Context, appdeploy *appdeployv1.AppDeploy, reconcileErr error) error {
	now := metav1.Now()
	readyCondition := metav1.Condition{
		Type:               "Ready",
		ObservedGeneration: appdeploy.Generation,
		LastTransitionTime: now,
	}
	degradedCondition := metav1.Condition{
		Type:               "Degraded",
		ObservedGeneration: appdeploy.Generation,
		LastTransitionTime: now,
	}

	if reconcileErr != nil {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = statusReasonForError(reconcileErr)
		readyCondition.Message = reconcileErr.Error()
		degradedCondition.Status = metav1.ConditionTrue
		degradedCondition.Reason = statusReasonForError(reconcileErr)
		degradedCondition.Message = reconcileErr.Error()
	} else {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "Reconciled"
		readyCondition.Message = "All desired resources were reconciled"
		degradedCondition.Status = metav1.ConditionFalse
		degradedCondition.Reason = "Reconciled"
		degradedCondition.Message = "All desired resources were reconciled"
	}

	appdeploy.Status.Conditions = upsertCondition(appdeploy.Status.Conditions, readyCondition)
	appdeploy.Status.Conditions = upsertCondition(appdeploy.Status.Conditions, degradedCondition)

	return r.Status().Update(ctx, appdeploy)
}

func statusReasonForError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "external secrets operator is not configured"):
		return "ESOUnavailable"
	case strings.HasPrefix(msg, "spec."):
		return "ValidationFailed"
	default:
		return "ReconcileFailed"
	}
}

func upsertCondition(conditions []metav1.Condition, condition metav1.Condition) []metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condition.Type {
			conditions[i] = condition
			return conditions
		}
	}
	return append(conditions, condition)
}

func (r *AppDeployReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appdeployv1.AppDeploy{}).
		Named("appdeploy").
		Complete(r)
}
