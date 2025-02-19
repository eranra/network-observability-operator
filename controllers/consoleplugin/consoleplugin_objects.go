package consoleplugin

import (
	"strings"

	osv1alpha1 "github.com/openshift/api/console/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flowsv1alpha1 "github.com/netobserv/network-observability-operator/api/v1alpha1"
	"github.com/netobserv/network-observability-operator/controllers/constants"
	"github.com/netobserv/network-observability-operator/pkg/helper"
)

const secretName = "console-serving-cert"
const displayName = "Network Observability plugin"
const proxyAlias = "backend"

// lokiURLAnnotation contains the used Loki querier URL, facilitating the change management
const lokiURLAnnotation = "flows.netobserv.io/loki-url"

type builder struct {
	namespace   string
	labels      map[string]string
	desired     *flowsv1alpha1.FlowCollectorConsolePlugin
	desiredLoki *flowsv1alpha1.FlowCollectorLoki
}

func newBuilder(ns string, desired *flowsv1alpha1.FlowCollectorConsolePlugin, desiredLoki *flowsv1alpha1.FlowCollectorLoki) builder {
	version := helper.ExtractVersion(desired.Image)
	return builder{
		namespace: ns,
		labels: map[string]string{
			"app":     pluginName,
			"version": version,
		},
		desired:     desired,
		desiredLoki: desiredLoki,
	}
}

func (b *builder) consolePlugin() *osv1alpha1.ConsolePlugin {
	return &osv1alpha1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name: pluginName,
		},
		Spec: osv1alpha1.ConsolePluginSpec{
			DisplayName: displayName,
			Service: osv1alpha1.ConsolePluginService{
				Name:      pluginName,
				Namespace: b.namespace,
				Port:      b.desired.Port,
				BasePath:  "/",
			},
			Proxy: []osv1alpha1.ConsolePluginProxy{{
				Type:  osv1alpha1.ProxyTypeService,
				Alias: proxyAlias,
				Service: osv1alpha1.ConsolePluginProxyServiceConfig{
					Name:      pluginName,
					Namespace: b.namespace,
					Port:      b.desired.Port,
				},
			}},
		},
	}
}

func (b *builder) deployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: b.namespace,
			Labels:    b.labels,
			Annotations: map[string]string{
				lokiURLAnnotation: querierURL(b.desiredLoki),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &b.desired.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: b.labels,
			},
			Template: *b.podTemplate(),
		},
	}
}

func (b *builder) podTemplate() *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: b.labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:            pluginName,
				Image:           b.desired.Image,
				ImagePullPolicy: corev1.PullPolicy(b.desired.ImagePullPolicy),
				Resources:       *b.desired.Resources.DeepCopy(),
				VolumeMounts: []corev1.VolumeMount{{
					Name:      secretName,
					MountPath: "/var/serving-cert",
					ReadOnly:  true,
				}},
				Args: []string{
					"-cert", "/var/serving-cert/tls.crt",
					"-key", "/var/serving-cert/tls.key",
					"-loki", querierURL(b.desiredLoki),
					"-loki-labels", strings.Join(constants.Labels, ","),
				},
			}},
			Volumes: []corev1.Volume{{
				Name: secretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName,
					},
				},
			}},
			ServiceAccountName: pluginName,
		},
	}
}

func (b *builder) service(old *corev1.Service) *corev1.Service {
	if old == nil {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pluginName,
				Namespace: b.namespace,
				Labels:    b.labels,
				Annotations: map[string]string{
					"service.alpha.openshift.io/serving-cert-secret-name": "console-serving-cert",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: b.labels,
				Ports: []corev1.ServicePort{{
					Port:     b.desired.Port,
					Protocol: "TCP",
				}},
			},
		}
	}
	// In case we're updating an existing service, we need to build from the old one to keep immutable fields such as clusterIP
	newService := old.DeepCopy()
	newService.Spec.Ports = []corev1.ServicePort{{
		Port:     b.desired.Port,
		Protocol: corev1.ProtocolUDP,
	}}
	return newService
}

func buildServiceAccount(ns string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: ns,
			Labels: map[string]string{
				"app": pluginName,
			},
		},
	}
}
