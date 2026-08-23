package render

import (
	"fmt"

	"github.com/ude-p/appdeploy/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildJobs(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Jobs))
	for _, job := range model.Jobs {
		if job.Scope != "" && job.Scope != namespace {
			continue
		}
		container, err := buildJobContainer(job.Name, job.Image, job.Command, job.Args, job.Env)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", job.Name, err)
		}
		mounts, volumes, err := buildMounts(job.Mounts)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", job.Name, err)
		}
		container.VolumeMounts = mounts

		objects = append(objects, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: namespace},
			Spec: batchv1.JobSpec{
				BackoffLimit: job.Retries,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{container},
					Volumes:       volumes,
					},
				},
			},
		})
	}
	return objects, nil
}

func buildCronJobs(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Crons))
	for _, cron := range model.Crons {
		if cron.Scope != "" && cron.Scope != namespace {
			continue
		}
		container, err := buildJobContainer(cron.Name, cron.Image, cron.Command, cron.Args, cron.Env)
		if err != nil {
			return nil, fmt.Errorf("cron %q: %w", cron.Name, err)
		}

		objects = append(objects, &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: cron.Name, Namespace: namespace},
			Spec: batchv1.CronJobSpec{
				Schedule: cron.Schedule,
				JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
					BackoffLimit: cron.Retries,
					Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers:    []corev1.Container{container},
					}},
				}},
			},
		})
	}
	return objects, nil
}

func buildJobContainer(name, image string, command, args []string, env *config.Env) (corev1.Container, error) {
	container := corev1.Container{Name: name, Image: image, ImagePullPolicy: corev1.PullAlways, Command: command, Args: args}
	envFrom, fields, err := buildEnv(env)
	if err != nil {
		return corev1.Container{}, err
	}
	container.EnvFrom = envFrom
	container.Env = fields
	return container, nil
}
