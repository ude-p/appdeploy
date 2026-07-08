/*
Copyright 2026.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appdeployv1alpha1 "github.com/ude-p/appdeploy/api/v1alpha1"
)

var _ = Describe("AppDeploy Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		appdeploy := &appdeployv1alpha1.AppDeploy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AppDeploy")
			err := k8sClient.Get(ctx, typeNamespacedName, appdeploy)
			if err != nil && errors.IsNotFound(err) {
				resource := &appdeployv1alpha1.AppDeploy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: appdeployv1alpha1.AppDeploySpec{
						Namespaces: []string{resourceNamespace},
						Workloads: []appdeployv1alpha1.AppDeployWorkload{
							{
								Name:          "app",
								Image:         "example.com/app-image:tag",
								ContainerPort: ptr.To[int32](8090),
								ServiceType:   string(corev1.ServiceTypeClusterIP),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &appdeployv1alpha1.AppDeploy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AppDeploy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AppDeployReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app", Namespace: resourceNamespace}, deployment)).To(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("example.com/app-image:tag"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8090)))

			service := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app", Namespace: resourceNamespace}, service)).To(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8090)))
			Expect(service.Spec.Ports[0].TargetPort.IntValue()).To(Equal(8090))
		})
	})

	Context("When reconciling ConfigMaps across namespaces", func() {
		const (
			resourceName      = "configmap-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		appdeploy := &appdeployv1alpha1.AppDeploy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AppDeploy")
			err := k8sClient.Get(ctx, typeNamespacedName, appdeploy)
			if err != nil && errors.IsNotFound(err) {
				resource := &appdeployv1alpha1.AppDeploy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: appdeployv1alpha1.AppDeploySpec{
						Namespaces: []string{resourceNamespace, "staging"},
						SelectedNamespaces: []string{"staging"},
						ConfigMaps: []appdeployv1alpha1.AppDeployConfigMap{
							{
								Name: "common-config",
								Data: map[string]string{
									"HOST": "0.0.0.0",
								},
							},
							{
								Name:  "app-config",
								Scope: "default",
								Data: map[string]string{
									"APP_ENV": "prod",
								},
							},
							{
								Name:  "app-config",
								Scope: "staging",
								Data: map[string]string{
									"APP_ENV": "staging",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &appdeployv1alpha1.AppDeploy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AppDeploy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should reconcile ConfigMaps only in the selected namespace", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AppDeployReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			commonConfig := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "common-config", Namespace: "staging"}, commonConfig)).To(Succeed())
			Expect(commonConfig.Data).To(HaveKeyWithValue("HOST", "0.0.0.0"))

			stagingConfig := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "staging"}, stagingConfig)).To(Succeed())
			Expect(stagingConfig.Data).To(HaveKeyWithValue("APP_ENV", "staging"))

			defaultConfig := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "common-config", Namespace: "default"}, defaultConfig)).NotTo(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "default"}, defaultConfig)).NotTo(Succeed())
		})
	})

	Context("When reconciling a StatefulSet workload", func() {
		const (
			resourceName      = "stateful-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		appdeploy := &appdeployv1alpha1.AppDeploy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AppDeploy")
			err := k8sClient.Get(ctx, typeNamespacedName, appdeploy)
			if err != nil && errors.IsNotFound(err) {
				resource := &appdeployv1alpha1.AppDeploy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: appdeployv1alpha1.AppDeploySpec{
						Namespaces: []string{resourceNamespace},
						Workloads: []appdeployv1alpha1.AppDeployWorkload{
							{
								Name:                "db",
								Kind:                "StatefulSet",
								Image:               "example.com/db-image:tag",
								ContainerPort:       ptr.To[int32](5432),
								HeadlessServiceName: "db-headless",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &appdeployv1alpha1.AppDeploy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AppDeploy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the StatefulSet workload", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AppDeployReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			statefulSet := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "db", Namespace: resourceNamespace}, statefulSet)).To(Succeed())
			Expect(statefulSet.Spec.ServiceName).To(Equal("db-headless"))
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("example.com/db-image:tag"))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(5432)))

			headlessService := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "db-headless", Namespace: resourceNamespace}, headlessService)).To(Succeed())
			Expect(headlessService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(headlessService.Spec.Ports).To(HaveLen(1))
			Expect(headlessService.Spec.Ports[0].Port).To(Equal(int32(5432)))
		})
	})

	Context("When reconciling a Job workload", func() {
		const (
			resourceName      = "job-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		appdeploy := &appdeployv1alpha1.AppDeploy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AppDeploy")
			err := k8sClient.Get(ctx, typeNamespacedName, appdeploy)
			if err != nil && errors.IsNotFound(err) {
				resource := &appdeployv1alpha1.AppDeploy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: appdeployv1alpha1.AppDeploySpec{
						Namespaces: []string{resourceNamespace},
						Workloads: []appdeployv1alpha1.AppDeployWorkload{
							{
								Name:                    "db-init",
								Kind:                    "Job",
								Image:                   "postgres:17",
								Command:                 []string{"sh", "-c"},
								Args:                    []string{"echo ok"},
								BackoffLimit:            ptr.To[int32](3),
								TTLSecondsAfterFinished: ptr.To[int32](60),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &appdeployv1alpha1.AppDeploy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AppDeploy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the Job workload", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AppDeployReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			job := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "db-init", Namespace: resourceNamespace}, job)).To(Succeed())
			Expect(job.Spec.BackoffLimit).NotTo(BeNil())
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(3)))
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"sh", "-c"}))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"echo ok"}))
		})
	})
})
