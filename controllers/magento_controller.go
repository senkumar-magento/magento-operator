/*


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

package controllers

import (
	"context"
	"github.com/prometheus/common/log"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	adobecomv1alpha1 "github.com/senkumar-magento/magento-operator.git/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MagentoReconciler reconciles a Magento object
type MagentoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=adobe.com.adobe.com,resources=magentoes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=adobe.com.adobe.com,resources=magentoes/status,verbs=get;update;patch

func (r *MagentoReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("magento", req.NamespacedName)

	// Fetch the Magento intance
	magento := &adobecomv1alpha1.Magento{}

	err := r.Get(ctx, req.NamespacedName, magento)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Magento resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Magento")
		return ctrl.Result{}, err
	}

	if err := r.reconcileResources(magento); err != nil {
		// Error reconciling Magento sub-resources - requeue the request.
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MagentoReconciler) reconcileResources(m *adobecomv1alpha1.Magento) error {
	// Reconcile Deployments
	r.Log.Info("Reconciling Deployments")
	if err := r.reconcileDeployments(m); err != nil {
		return err
	}

	// Placeholder for reconciling other resources
	return nil
}

func (r *MagentoReconciler) reconcileDeployments(m *adobecomv1alpha1.Magento) error {
	// Reconcile Magento App deployment
	err := r.reconcileMagentoApp(m)
	if err != nil {
		return err
	}

	// Placeholder for reconciling other deployments
	return nil
}

func (r *MagentoReconciler) reconcileMagentoApp(m *adobecomv1alpha1.Magento) error {
	// Check if the magento-app deployment already exists, if not create a new one
	magentoAppDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "magento-app",
			Namespace: m.Namespace,
		},
	}

	err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: m.Namespace, Name: "magento-app"}, magentoAppDeployment)
	if err != nil && errors.IsNotFound(err) {
		// Define a new magento-app deployment
		dep := r.deploymentForMagentoApp(m)
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.name", dep.Name)
		err := r.Create(context.TODO(), dep)
		if err != nil {
			log.Error(err, "Failed to create a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.name", dep.Name)

			return err
		}
		// Deployment created successfully - return and requeue
		return nil

	} else if err != nil {
		log.Error(err, "Failed to get Deployment")

		return err
	}

	size := m.Spec.MagentoPhp.Replicas
	if *magentoAppDeployment.Spec.Replicas != size {
		magentoAppDeployment.Spec.Replicas = &size
		err := r.Update(context.TODO(), magentoAppDeployment)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", magentoAppDeployment.Namespace, "Deployment.Name", magentoAppDeployment.Name)
			return err
		}
		// Spec updated - return and requeue
		return nil
	}

	return nil
}

func (r *MagentoReconciler) deploymentForMagentoApp(m *adobecomv1alpha1.Magento) *appsv1.Deployment {
	ls := labelsForMagentoApp(m.Name)
	replicas := m.Spec.MagentoPhp.Replicas

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "magento-app",
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: m.Spec.MagentoPhp.Image,
						Name:  "magento-app",
						Ports: []corev1.ContainerPort{{
							/*
								TODO: This is only exposing the php-fpm port of magento-cloud image
								eventually, when we switch to a pre-canned Magento image it should expose port 80 instead
							*/
							ContainerPort: 9000,
							Name:          "php-fpm",
						}},
					}},
				},
			},
		},
	}
	// Set Magento instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

func labelsForMagentoApp(name string) map[string]string {
	return map[string]string{"app": "magento-app", "magento_cr": name}
}

func (r *MagentoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&adobecomv1alpha1.Magento{}).
		Complete(r)
}
