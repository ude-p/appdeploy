package controller

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appdeployv1 "github.com/ude-p/appdeploy/api/v1"
)

var deploymentOverrideAllowlist = map[string]struct{}{
	"metadata.annotations":               {},
	"metadata.labels":                    {},
	"spec.strategy":                      {},
	"spec.template.metadata.annotations": {},
	"spec.template.spec.securityContext": {},
}

func (r *AppDeployReconciler) reconcileDeployment(ctx context.Context, namespace string, workload *appdeployv1.AppDeployWorkload) error {
	name := workload.Name
	replicas := int32(1)
	if workload.Replicas != nil {
		replicas = *workload.Replicas
	}
	ports := workloadPorts(workload)
	containerPorts := buildContainerPorts(ports)
	envFrom := buildEnvFromSources(workload)
	imagePullSecrets := buildImagePullSecrets(workload)
	policy := imagePullPolicy(workload)
	volumeMounts := buildVolumeMounts(workload)
	volumes := buildVolumes(workload)
	resources := workload.Resources

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	err := r.Get(ctx, key, deployment)
	if apierrors.IsNotFound(err) {
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appdeploy.io/workload": name,
					},
				},
					Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"appdeploy.io/workload": name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            name,
								Image:           workload.Image,
								ImagePullPolicy: policy,
								EnvFrom:         envFrom,
								VolumeMounts:    volumeMounts,
								Resources:       resources,
								Ports:           containerPorts,
							},
						},
						ImagePullSecrets: imagePullSecrets,
						Volumes:          volumes,
					},
				},
			},
		}
		if err := applyDeploymentOverrides(deployment, workload.Overrides.Raw); err != nil {
			return err
		}
		return r.Create(ctx, deployment)
	}
	if err != nil {
		return err
	}

	deployment.Spec.Replicas = &replicas
	deployment.Spec.Template.Labels = map[string]string{
		"appdeploy.io/workload": name,
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{Name: name}}
	}
	deployment.Spec.Template.Spec.Containers[0].Name = name
	deployment.Spec.Template.Spec.Containers[0].Image = workload.Image
	deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = envFrom
	deployment.Spec.Template.Spec.Containers[0].VolumeMounts = volumeMounts
	deployment.Spec.Template.Spec.Containers[0].Resources = resources
	deployment.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets
	deployment.Spec.Template.Spec.Volumes = volumes
	if err := applyDeploymentOverrides(deployment, workload.Overrides.Raw); err != nil {
		return err
	}
	deployment.Spec.Template.Spec.Containers[0].Ports = containerPorts

	return r.Update(ctx, deployment)
}

func (r *AppDeployReconciler) reconcileService(ctx context.Context, namespace string, workload *appdeployv1.AppDeployWorkload) error {
	if workload.ServiceType == "" {
		return nil
	}

	ports := workloadPorts(workload)
	servicePorts := buildServicePorts(ports)

	service := &corev1.Service{}
	key := client.ObjectKey{Name: workload.Name, Namespace: namespace}
	err := r.Get(ctx, key, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workload.Name,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceType(workload.ServiceType),
				Selector: map[string]string{
					"appdeploy.io/workload": workload.Name,
				},
				Ports: servicePorts,
			},
		}
		return r.Create(ctx, service)
	}
	if err != nil {
		return err
	}

	if service.Spec.Type != corev1.ServiceTypeExternalName {
		service.Spec.Type = corev1.ServiceType(workload.ServiceType)
	}
	service.Spec.Selector = map[string]string{
		"appdeploy.io/workload": workload.Name,
	}
	service.Spec.Ports = servicePorts

	return r.Update(ctx, service)
}

func (r *AppDeployReconciler) reconcileStatefulSet(ctx context.Context, namespace string, workload *appdeployv1.AppDeployWorkload) error {
	name := workload.Name
	replicas := int32(1)
	if workload.Replicas != nil {
		replicas = *workload.Replicas
	}
	ports := workloadPorts(workload)
	containerPorts := buildContainerPorts(ports)
	servicePorts := buildServicePorts(ports)
	envFrom := buildEnvFromSources(workload)
	imagePullSecrets := buildImagePullSecrets(workload)
	policy := imagePullPolicy(workload)
	volumeMounts := buildVolumeMounts(workload)
	volumes := buildVolumes(workload)
	volumeClaimTemplates := buildVolumeClaimTemplates(workload)
	resources := workload.Resources

	serviceName := workload.HeadlessServiceName
	if serviceName == "" {
		serviceName = name
	}

	if err := r.reconcileHeadlessService(ctx, namespace, serviceName, name, servicePorts); err != nil {
		return err
	}

	statefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	err := r.Get(ctx, key, statefulSet)
	if apierrors.IsNotFound(err) {
		statefulSet = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: serviceName,
					Replicas:    &replicas,
					Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
							"appdeploy.io/workload": name,
						},
					},
					VolumeClaimTemplates: volumeClaimTemplates,
					Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"appdeploy.io/workload": name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            name,
								Image:           workload.Image,
								ImagePullPolicy: policy,
								EnvFrom:         envFrom,
								VolumeMounts:    volumeMounts,
								Resources:       resources,
								Ports:           containerPorts,
							},
						},
						ImagePullSecrets: imagePullSecrets,
						Volumes:          volumes,
					},
				},
			},
		}
		return r.Create(ctx, statefulSet)
	}
	if err != nil {
		return err
	}

	statefulSet.Spec.Replicas = &replicas
	statefulSet.Spec.Template.Labels = map[string]string{
		"appdeploy.io/workload": name,
	}
	if len(statefulSet.Spec.Template.Spec.Containers) == 0 {
		statefulSet.Spec.Template.Spec.Containers = []corev1.Container{{Name: name}}
	}
	statefulSet.Spec.Template.Spec.Containers[0].Name = name
	statefulSet.Spec.Template.Spec.Containers[0].Image = workload.Image
	statefulSet.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	statefulSet.Spec.Template.Spec.Containers[0].EnvFrom = envFrom
	statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = volumeMounts
	statefulSet.Spec.Template.Spec.Containers[0].Resources = resources
	statefulSet.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets
	statefulSet.Spec.Template.Spec.Volumes = volumes
	statefulSet.Spec.Template.Spec.Containers[0].Ports = containerPorts
	statefulSet.Spec.VolumeClaimTemplates = volumeClaimTemplates

	return r.Update(ctx, statefulSet)
}

func (r *AppDeployReconciler) reconcileJob(ctx context.Context, namespace string, workload *appdeployv1.AppDeployWorkload) error {
	name := workload.Name
	backoffLimit := int32(6)
	if workload.BackoffLimit != nil {
		backoffLimit = *workload.BackoffLimit
	}
	envFrom := buildEnvFromSources(workload)
	imagePullSecrets := buildImagePullSecrets(workload)
	policy := imagePullPolicy(workload)
	volumeMounts := buildVolumeMounts(workload)
	volumes := buildVolumes(workload)
	resources := workload.Resources

	job := &batchv1.Job{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	err := r.Get(ctx, key, job)
	if apierrors.IsNotFound(err) {
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: batchv1.JobSpec{
				BackoffLimit:            &backoffLimit,
				TTLSecondsAfterFinished: workload.TTLSecondsAfterFinished,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"appdeploy.io/workload": name,
						},
					},
					Spec: corev1.PodSpec{
						RestartPolicy:    corev1.RestartPolicyOnFailure,
						ImagePullSecrets: imagePullSecrets,
						Volumes:          volumes,
						Containers: []corev1.Container{
							{
								Name:            name,
								Image:           workload.Image,
								ImagePullPolicy: policy,
								Command:         workload.Command,
								Args:            workload.Args,
								EnvFrom:         envFrom,
								VolumeMounts:    volumeMounts,
								Resources:       resources,
							},
						},
					},
				},
			},
		}
		return r.Create(ctx, job)
	}
	if err != nil {
		return err
	}

	job.Spec.BackoffLimit = &backoffLimit
	job.Spec.TTLSecondsAfterFinished = workload.TTLSecondsAfterFinished
	job.Spec.Template.Labels = map[string]string{
		"appdeploy.io/workload": name,
	}
	if len(job.Spec.Template.Spec.Containers) == 0 {
		job.Spec.Template.Spec.Containers = []corev1.Container{{Name: name}}
	}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	job.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets
	job.Spec.Template.Spec.Volumes = volumes
	job.Spec.Template.Spec.Containers[0].Name = name
	job.Spec.Template.Spec.Containers[0].Image = workload.Image
	job.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	job.Spec.Template.Spec.Containers[0].Command = workload.Command
	job.Spec.Template.Spec.Containers[0].Args = workload.Args
	job.Spec.Template.Spec.Containers[0].EnvFrom = envFrom
	job.Spec.Template.Spec.Containers[0].VolumeMounts = volumeMounts
	job.Spec.Template.Spec.Containers[0].Resources = resources

	return r.Update(ctx, job)
}

func (r *AppDeployReconciler) reconcileHeadlessService(ctx context.Context, namespace, serviceName, workloadName string, servicePorts []corev1.ServicePort) error {
	service := &corev1.Service{}
	key := client.ObjectKey{Name: serviceName, Namespace: namespace}
	err := r.Get(ctx, key, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector: map[string]string{
					"appdeploy.io/workload": workloadName,
				},
				Ports: servicePorts,
			},
		}
		return r.Create(ctx, service)
	}
	if err != nil {
		return err
	}

	service.Spec.Selector = map[string]string{
		"appdeploy.io/workload": workloadName,
	}
	service.Spec.Ports = servicePorts

	return r.Update(ctx, service)
}

type workloadPort struct {
	servicePort   int32
	containerPort int32
}

func workloadPorts(workload *appdeployv1.AppDeployWorkload) []workloadPort {
	ports := make([]workloadPort, len(workload.ServicePorts))
	for i, servicePort := range workload.ServicePorts {
		containerPort := servicePort
		if len(workload.ContainerPorts) > 0 {
			containerPort = workload.ContainerPorts[i]
		}
		ports[i] = workloadPort{
			servicePort:   servicePort,
			containerPort: containerPort,
		}
	}
	return ports
}

func buildContainerPorts(ports []workloadPort) []corev1.ContainerPort {
	containerPorts := make([]corev1.ContainerPort, 0, len(ports))
	seen := make(map[int32]struct{}, len(ports))
	for _, port := range ports {
		if _, ok := seen[port.containerPort]; ok {
			continue
		}
		seen[port.containerPort] = struct{}{}
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: port.containerPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return containerPorts
}

func buildServicePorts(ports []workloadPort) []corev1.ServicePort {
	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", port.servicePort),
			Port:       port.servicePort,
			TargetPort: intstr.FromInt(int(port.containerPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return servicePorts
}

func applyDeploymentOverrides(deployment *appsv1.Deployment, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return fmt.Errorf("invalid deployment overrides: %w", err)
	}

	for key := range overrides {
		if _, ok := deploymentOverrideAllowlist[key]; !ok {
			return fmt.Errorf("deployment override path %q is not allowed", key)
		}
	}

	for key, value := range overrides {
		switch key {
		case "metadata.annotations":
			if err := json.Unmarshal(value, &deployment.Annotations); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "metadata.labels":
			if err := json.Unmarshal(value, &deployment.Labels); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.strategy":
			if err := json.Unmarshal(value, &deployment.Spec.Strategy); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.template.metadata.annotations":
			if err := json.Unmarshal(value, &deployment.Spec.Template.Annotations); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.template.spec.securityContext":
			var securityContext corev1.PodSecurityContext
			if err := json.Unmarshal(value, &securityContext); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
			deployment.Spec.Template.Spec.SecurityContext = &securityContext
		}
	}

	return nil
}
