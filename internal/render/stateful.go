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

func buildStatefulSets(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Statefuls)*2)
	for _, stateful := range model.Statefuls {
		if stateful.Scope != "" && stateful.Scope != namespace {
			continue
		}
		labels := map[string]string{"app.kubernetes.io/name": stateful.Name}
		replicas := int32(1)
		container := corev1.Container{Name: stateful.Name, Image: stateful.Image, ImagePullPolicy: corev1.PullAlways}

		if stateful.Env != nil {
			envFrom, env, err := buildEnv(stateful.Env)
			if err != nil {
				return nil, fmt.Errorf("stateful %q: %w", stateful.Name, err)
			}
			container.EnvFrom = envFrom
			container.Env = env
		}

		for _, port := range stateful.Ports {
			containerPort := port.Container
			if containerPort == 0 {
				containerPort = port.Service
			}
			container.Ports = append(container.Ports, corev1.ContainerPort{ContainerPort: containerPort})
		}
		mounts, volumes, err := buildMounts(stateful.Mounts)
		if err != nil {
			return nil, fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
		container.VolumeMounts = append(container.VolumeMounts, mounts...)

		claimTemplates := make([]corev1.PersistentVolumeClaim, 0, len(stateful.Volumes))
		for _, volume := range stateful.Volumes {
			size, err := resource.ParseQuantity(volume.Size)
			if err != nil {
				return nil, fmt.Errorf("stateful %q volume %q: parse size: %w", stateful.Name, volume.Name, err)
			}
			claim := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: volume.Name},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(volume.Access)},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: size},
					},
				},
			}
			if volume.StorageClass != "" {
				claim.Spec.StorageClassName = &volume.StorageClass
			}
			claimTemplates = append(claimTemplates, claim)
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: volume.Name, MountPath: volume.Path})
		}

		objects = append(objects, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: stateful.Name, Namespace: namespace},
			Spec: appsv1.StatefulSetSpec{
				ServiceName:          stateful.Name,
				Replicas:             &replicas,
				Selector:             &metav1.LabelSelector{MatchLabels: labels},
				VolumeClaimTemplates: claimTemplates,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{container}, Volumes: volumes},
				},
			},
		})

		ports := make([]corev1.ServicePort, 0, len(stateful.Ports))
		for _, port := range stateful.Ports {
			containerPort := port.Container
			if containerPort == 0 {
				containerPort = port.Service
			}
			ports = append(ports, corev1.ServicePort{Port: port.Service, TargetPort: intstr.FromInt32(containerPort)})
		}
		objects = append(objects, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: stateful.Name, Namespace: namespace},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector:  labels,
				Ports:     ports,
			},
		})
	}
	return objects, nil
}
