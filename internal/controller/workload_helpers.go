package controller

import (
	corev1 "k8s.io/api/core/v1"

	appdeployv1 "github.com/ude-p/appdeploy/api/v1"
)

func buildEnvFromSources(workload *appdeployv1.AppDeployWorkload) []corev1.EnvFromSource {
	envFrom := make([]corev1.EnvFromSource, 0, len(workload.EnvFromConfig)+len(workload.EnvFromSecrets))
	for _, configMapName := range workload.EnvFromConfig {
		envFrom = append(envFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		})
	}
	for _, secretName := range workload.EnvFromSecrets {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		})
	}
	return envFrom
}

func imagePullPolicy(workload *appdeployv1.AppDeployWorkload) corev1.PullPolicy {
	if workload.ImagePullPolicy != "" {
		return corev1.PullPolicy(workload.ImagePullPolicy)
	}
	return corev1.PullIfNotPresent
}

func buildImagePullSecrets(workload *appdeployv1.AppDeployWorkload) []corev1.LocalObjectReference {
	if len(workload.ImagePullSecrets) == 0 {
		return nil
	}

	imagePullSecrets := make([]corev1.LocalObjectReference, 0, len(workload.ImagePullSecrets))
	for _, name := range workload.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return imagePullSecrets
}

func buildVolumeMounts(workload *appdeployv1.AppDeployWorkload) []corev1.VolumeMount {
	if len(workload.VolumeMounts) == 0 {
		return nil
	}

	volumeMounts := make([]corev1.VolumeMount, 0, len(workload.VolumeMounts))
	for _, mount := range workload.VolumeMounts {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      mount.Name,
			MountPath: mount.MountPath,
		})
	}
	return volumeMounts
}

func buildVolumes(workload *appdeployv1.AppDeployWorkload) []corev1.Volume {
	if len(workload.VolumeMounts) == 0 {
		return nil
	}

	volumes := make([]corev1.Volume, 0, len(workload.VolumeMounts))
	for _, mount := range workload.VolumeMounts {
		volume := corev1.Volume{
			Name: mount.Name,
		}
		if mount.ConfigMapName != "" {
			volume.VolumeSource.ConfigMap = &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: mount.ConfigMapName},
			}
		}
		if mount.SecretName != "" {
			volume.VolumeSource.Secret = &corev1.SecretVolumeSource{
				SecretName: mount.SecretName,
			}
		}
		if mount.PersistentVolumeClaimName != "" {
			volume.VolumeSource.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: mount.PersistentVolumeClaimName,
			}
		}
		volumes = append(volumes, volume)
	}
	return volumes
}
