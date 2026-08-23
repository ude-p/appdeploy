package render

import (
	"fmt"

	"github.com/ude-p/appdeploy/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildApps(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Apps)*2)
	for _, app := range model.Apps {
		labels := map[string]string{"app.kubernetes.io/name": app.Name}
		replicas := int32(1)
		if app.Replicas != nil {
			replicas = *app.Replicas
		}

		container, volumes, err := buildContainer(app)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", app.Name, err)
		}
		objects = append(objects, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: namespace},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{container},
						Volumes:    volumes,
					},
				},
			},
		})
		if len(app.Ports) > 0 {
			objects = append(objects, buildService(app, namespace))
		}
	}
	return objects, nil
}

func buildContainer(app config.App) (corev1.Container, []corev1.Volume, error) {
	container := corev1.Container{
		Name:            app.Name,
		Image:           app.Image,
		ImagePullPolicy: corev1.PullAlways,
		Command:         app.Command,
		Args:            app.Args,
	}
	if app.Env != nil {
		if app.Env.Config != nil {
			name, err := referenceName(app.Env.Config)
			if err != nil {
				return corev1.Container{}, nil, err
			}
			container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
			})
		}
		if app.Env.Secret != nil {
			name, err := referenceName(app.Env.Secret)
			if err != nil {
				return corev1.Container{}, nil, err
			}
			container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
			})
		}
	}

	for _, port := range app.Ports {
		containerPort := port.Container
		if containerPort == 0 {
			containerPort = port.Service
		}
		container.Ports = append(container.Ports, corev1.ContainerPort{ContainerPort: containerPort})
	}

	if app.Resources != nil {
		cpu, err := resource.ParseQuantity(app.Resources.CPU)
		if err != nil {
			return corev1.Container{}, nil, fmt.Errorf("parse CPU: %w", err)
		}
		memory, err := resource.ParseQuantity(app.Resources.Memory)
		if err != nil {
			return corev1.Container{}, nil, fmt.Errorf("parse memory: %w", err)
		}
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
		}
	}

	if app.Health != nil {
		probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: app.Health.Path, Port: intstr.FromInt32(app.Health.Port)},
		}}
		container.LivenessProbe = probe
		container.ReadinessProbe = probe.DeepCopy()
	}

	volumes := make([]corev1.Volume, 0, len(app.Mounts))
	for _, mount := range app.Mounts {
		name, err := referenceName(mount.Volume)
		if err != nil {
			return corev1.Container{}, nil, err
		}
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: name, MountPath: mount.Path})
		volumes = append(volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name},
			},
		})
	}

	return container, volumes, nil
}

func buildService(app config.App, namespace string) *corev1.Service {
	ports := make([]corev1.ServicePort, 0, len(app.Ports))
	for _, port := range app.Ports {
		containerPort := port.Container
		if containerPort == 0 {
			containerPort = port.Service
		}
		ports = append(ports, corev1.ServicePort{
			Port:       port.Service,
			TargetPort: intstr.FromInt32(containerPort),
		})
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": app.Name},
			Ports:    ports,
		},
	}
}
