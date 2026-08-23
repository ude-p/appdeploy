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
		if app.Scope != "" && app.Scope != namespace {
			continue
		}
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
						NodeSelector: app.NodeSelector,
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
		envFrom, env, err := buildEnv(app.Env)
		if err != nil {
			return corev1.Container{}, nil, err
		}
		container.EnvFrom = envFrom
		container.Env = env
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

	mounts, volumes, err := buildMounts(app.Mounts)
	if err != nil {
		return corev1.Container{}, nil, err
	}
	container.VolumeMounts = mounts

	return container, volumes, nil
}

func buildEnv(env *config.Env) ([]corev1.EnvFromSource, []corev1.EnvVar, error) {
	if env == nil {
		return nil, nil, nil
	}
	envFrom := make([]corev1.EnvFromSource, 0, 2)
	if config.HasExpression(env.Config) {
		name, err := referenceName(env.Config)
		if err != nil {
			return nil, nil, err
		}
		envFrom = append(envFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
		})
	}
	if config.HasExpression(env.Secret) {
		name, err := referenceName(env.Secret)
		if err != nil {
			return nil, nil, err
		}
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
		})
	}
	fields := make([]corev1.EnvVar, 0, len(env.Fields))
	for _, field := range env.Fields {
		fields = append(fields, corev1.EnvVar{
			Name: field.Name,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: field.Path},
			},
		})
	}
	return envFrom, fields, nil
}

func buildMounts(mounts []config.Mount) ([]corev1.VolumeMount, []corev1.Volume, error) {
	volumeMounts := make([]corev1.VolumeMount, 0, len(mounts))
	volumes := make([]corev1.Volume, 0, len(mounts))
	for _, mount := range mounts {
		var name string
		var source corev1.VolumeSource
		var err error
		switch {
		case config.HasExpression(mount.Volume):
			name, err = referenceName(mount.Volume)
			source.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name}
		case config.HasExpression(mount.Config):
			name, err = referenceName(mount.Config)
			source.ConfigMap = &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}
		case config.HasExpression(mount.Secret):
			name, err = referenceName(mount.Secret)
			source.Secret = &corev1.SecretVolumeSource{SecretName: name}
		}
		if err != nil {
			return nil, nil, err
		}
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: name, MountPath: mount.Path})
		volumes = append(volumes, corev1.Volume{Name: name, VolumeSource: source})
	}
	return volumeMounts, volumes, nil
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
