// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	b64 "encoding/base64"
	"fmt"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/garbagecollector/references"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Actuator", func() {

	DescribeTable("Should get correct number of replicas",
		func(hibernated bool) {
			expectedReplicas := 3
			if hibernated {
				expectedReplicas = 0
			}

			actual := getLakomReplicas(hibernated)
			Expect(*actual).To(BeEquivalentTo(expectedReplicas))
		},
		Entry("Awaken", false),
		Entry("Hibernated", true),
	)

	It("Should get labels", func() {
		labels := getLabels()

		Expect(labels).To(HaveLen(2))
		appName, appNameOK := labels["app.kubernetes.io/name"]
		Expect(appNameOK).To(BeTrue())
		Expect(appName).To(Equal("lakom"))

		appPartOf, appPartOfOK := labels["app.kubernetes.io/part-of"]
		Expect(appPartOfOK).To(BeTrue())
		Expect(appPartOf).To(Equal("shoot-lakom-service"))
	})

	Context("getPDB", func() {
		It("Should return PDB even when replicas is zero or unset", func() {
			var (
				replicas  *int32
				namespace = "default"
			)

			version, err := semver.NewVersion("v1.22.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(version).ToNot(BeNil())

			pdb, err := getPDB(replicas, namespace, version)
			Expect(err).ToNot(HaveOccurred())
			Expect(pdb).ToNot(BeNil())

			policyv1PDB, ok := pdb.(*policyv1.PodDisruptionBudget)
			Expect(ok).To(BeTrue())
			Expect(policyv1PDB.Spec.MaxUnavailable.IntValue()).To(Equal(1))

			version, err = semver.NewVersion("v1.20.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(version).ToNot(BeNil())

			replicas = pointer.Int32(0)
			pdb, err = getPDB(replicas, namespace, version)
			Expect(err).ToNot(HaveOccurred())
			Expect(pdb).ToNot(BeNil())

			policyv1beta1PDB, ok := pdb.(*policyv1beta1.PodDisruptionBudget)
			Expect(ok).To(BeTrue())
			Expect(policyv1beta1PDB.Spec.MaxUnavailable.IntValue()).To(Equal(1))
		})

		DescribeTable("Should use the right apiVersion for PodDisruptionBudgets depending on k8s version",
			func(k8sVersion string, expectedType interface{}) {
				var (
					replicas  = pointer.Int32(3)
					namespace = "default"
				)

				version, err := semver.NewVersion(k8sVersion)
				Expect(err).ToNot(HaveOccurred())

				pdb, err := getPDB(replicas, namespace, version)
				Expect(err).ToNot(HaveOccurred())
				Expect(pdb).ToNot(BeNil())
				Expect(pdb).To(BeAssignableToTypeOf(expectedType))

			},
			Entry("Should use policy/v1beta1 for 1.19.0", "1.19.0", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.19.0", "v1.19.0", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.20.0", "v1.20.0", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.20.1", "v1.20.1", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.20.0-gke.100", "v1.20.0-gke.100", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.20.0-0.0.0", "v1.20.0-0.0.0", &policyv1beta1.PodDisruptionBudget{}),
			Entry("Should use policy/v1beta1 for v1.20.1-0.0.0", "v1.20.1-0.0.0", &policyv1beta1.PodDisruptionBudget{}),

			Entry("Should use policy/v1 for 1.21.0", "1.21.0", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.21.0", "v1.21.0", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.21.1", "v1.21.1", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.21.0-gke.100", "v1.21.0-gke.100", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.21.0-0.0.0", "v1.21.0-0.0.0", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.21.1-0.0.0", "v1.21.1-0.0.0", &policyv1.PodDisruptionBudget{}),
			Entry("Should use policy/v1 for v1.22.0", "v1.22.0", &policyv1.PodDisruptionBudget{}),
		)
	})

	Context("getShootResources", func() {
		const (
			namespace                     = "shoot--for--bar"
			shootAccessServiceAccountName = "extension-shoot-lakom-service-access"
			failurePolicy                 = admissionregistrationv1.Ignore
			validatingWebhookKey          = "validatingwebhookconfiguration____gardener-extension-shoot-lakom-service-shoot.yaml"
			mutatingWebhookKey            = "mutatingwebhookconfiguration____gardener-extension-shoot-lakom-service-shoot.yaml"
			roleKey                       = "role__kube-system__gardener-extension-shoot-lakom-service-resource-reader.yaml"
			roleBindingKey                = "rolebinding__kube-system__gardener-extension-shoot-lakom-service-resource-reader.yaml"
		)
		var (
			caBundle = []byte("caBundle")
		)

		It("Should ensure the correct shoot resources are created", func() {

			resources, err := getShootResources(caBundle, namespace, shootAccessServiceAccountName, failurePolicy)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(4))

			Expect(resources).To(Equal(map[string][]byte{
				validatingWebhookKey: []byte(expectedSeedValidatingWebhook(caBundle, namespace, failurePolicy)),
				mutatingWebhookKey:   []byte(expectedShootMutatingWebhook(caBundle, namespace, failurePolicy)),
				roleKey:              []byte(expectedShootRole()),
				roleBindingKey:       []byte(expectedShootRoleBinding(shootAccessServiceAccountName)),
			}))
		})

		DescribeTable("Should ensure the mutating webhook config is correctly set",
			func(ca []byte, ns string, fp admissionregistrationv1.FailurePolicyType) {
				resources, err := getShootResources(ca, ns, shootAccessServiceAccountName, fp)
				Expect(err).ToNot(HaveOccurred())

				mutatingWebhook, ok := resources[mutatingWebhookKey]
				Expect(ok).To(BeTrue())
				Expect(string(mutatingWebhook)).To(Equal(expectedShootMutatingWebhook(ca, ns, fp)))
			},
			Entry("Failure policy Fail", caBundle, namespace, admissionregistrationv1.Fail),
			Entry("Failure policy Ignore", []byte("anotherCABundle"), "different-namespace", admissionregistrationv1.Ignore),
		)

		DescribeTable("Should ensure the validating webhook config is correctly set",
			func(ca []byte, ns string, fp admissionregistrationv1.FailurePolicyType) {
				resources, err := getShootResources(ca, ns, shootAccessServiceAccountName, fp)
				Expect(err).ToNot(HaveOccurred())

				validatingWebhook, ok := resources[validatingWebhookKey]
				Expect(ok).To(BeTrue())
				Expect(string(validatingWebhook)).To(Equal(expectedSeedValidatingWebhook(ca, ns, fp)))
			},
			Entry("Failure policy Fail", caBundle, namespace, admissionregistrationv1.Fail),
			Entry("Failure policy Ignore", []byte("anotherCABundle"), "different-namespace", admissionregistrationv1.Ignore),
		)

		DescribeTable("Should ensure the rolebinding is correctly set",
			func(saName string) {
				resources, err := getShootResources(caBundle, namespace, saName, failurePolicy)
				Expect(err).ToNot(HaveOccurred())

				roleBinding, ok := resources[roleBindingKey]
				Expect(ok).To(BeTrue())
				Expect(string(roleBinding)).To(Equal(expectedShootRoleBinding(saName)))
			},
			Entry("ServiceAccount name: test", "test"),
			Entry("ServiceAccount name: foo-bar", "foo-bar"),
		)

	})

	Context("getSeedResources", func() {
		const (
			namespace                     = "shoot--for--bar"
			genericKubeconfigName         = "generic-kubeconfig"
			shootAccessServiceAccountName = "extension-shoot-lakom-service"
			serverTLSSecretName           = "shoot-lakom-service-tls" //#nosec G101 -- this is false positive
			image                         = "eu.gcr.io/gardener-project/gardener/extensions/lakom:v0.0.0"
			cosignSecretName              = "extension-shoot-lakom-service-cosign-public-keys-e3b0c442"

			cosignSecretNameKey = "secret__" + namespace + "__" + cosignSecretName + ".yaml"
			configMapKey        = "configmap__" + namespace + "__extension-shoot-lakom-service-monitoring.yaml"
			deploymentKey       = "deployment__" + namespace + "__extension-shoot-lakom-service.yaml"
			pdbKey              = "poddisruptionbudget__" + namespace + "__extension-shoot-lakom-service.yaml"
			serviceKey          = "service__" + namespace + "__extension-shoot-lakom-service.yaml"
			serviceAccountKey   = "serviceaccount__" + namespace + "__extension-shoot-lakom-service.yaml"
			vpaKey              = "verticalpodautoscaler__" + namespace + "__extension-shoot-lakom-service.yaml"
		)

		var (
			replicas         int32
			cosignPublicKeys []string
			seedK8SVersion   *semver.Version
		)

		BeforeEach(func() {
			replicas = int32(3)

			cosignPublicKeys = []string{
				`-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5WIqxApep8Q53M5zrd0Hhuk03tCn
On/cxJW6vXn3mvlqgyc4MO/ZXb5EputelfyP5n1NYWWcomeQTDG/E3EbdQ==
-----END PUBLIC KEY-----
`, `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEyLVOS/TWANf6sZJPDzogodvDz8NT
hjZVcW2ygAvImCAULGph2fqGkNUszl7ycJH/Dntw4wMLSbstUZomqPuIVQ==
-----END PUBLIC KEY-----
`,
			}

			var err error
			seedK8SVersion, err = semver.NewVersion("v1.24.0")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should ensure the correct seed resources are created", func() {
			resources, err := getSeedResources(
				&replicas,
				namespace,
				genericKubeconfigName,
				shootAccessServiceAccountName,
				serverTLSSecretName,
				cosignPublicKeys,
				image,
				seedK8SVersion,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(7))

			expectedResources := map[string]string{
				configMapKey:        expectedSeedConfigMap(namespace),
				deploymentKey:       expectedSeedDeployment(replicas, namespace, genericKubeconfigName, shootAccessServiceAccountName, image, cosignSecretName, serverTLSSecretName),
				pdbKey:              expectedSeedPDB(namespace),
				cosignSecretNameKey: expectedSeedSecretCosign(namespace, cosignSecretName, cosignPublicKeys),
				serviceKey:          expectedSeedService(namespace),
				serviceAccountKey:   expectedSeedServiceAccount(namespace, shootAccessServiceAccountName),
				vpaKey:              expectedSeedVPA(namespace),
			}

			for key, expectedResource := range expectedResources {
				resource, ok := resources[key]
				Expect(ok).To(BeTrue())

				strResource := string(resource)
				Expect(strResource).To(Equal(expectedResource), key)
			}
		})
	})
})

func expectedShootMutatingWebhook(caBundle []byte, namespace string, failurePolicy admissionregistrationv1.FailurePolicyType) string {
	var (
		caBundleEncoded  = b64.StdEncoding.EncodeToString(caBundle)
		strFailurePolicy = string(failurePolicy)
	)

	return `apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
    remediation.webhook.shoot.gardener.cloud/exclude: "true"
  name: gardener-extension-shoot-lakom-service-shoot
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ` + caBundleEncoded + `
    url: https://extension-shoot-lakom-service.` + namespace + `/lakom/resolve-tag-to-digest
  failurePolicy: ` + strFailurePolicy + `
  matchPolicy: Equivalent
  name: resolve-tag.lakom.service.extensions.gardener.cloud
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - kube-system
  objectSelector:
    matchExpressions:
    - key: resources.gardener.cloud/managed-by
      operator: In
      values:
      - gardener
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - v1
    operations:
    - CREATE
    - UPDATE
    resources:
    - pods
    - pods/ephemeralcontainers
  sideEffects: None
  timeoutSeconds: 25
`
}

func expectedSeedValidatingWebhook(caBundle []byte, namespace string, failurePolicy admissionregistrationv1.FailurePolicyType) string {
	var (
		caBundleEncoded  = b64.StdEncoding.EncodeToString(caBundle)
		strFailurePolicy = string(failurePolicy)
	)
	return `apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
    remediation.webhook.shoot.gardener.cloud/exclude: "true"
  name: gardener-extension-shoot-lakom-service-shoot
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ` + caBundleEncoded + `
    url: https://extension-shoot-lakom-service.` + namespace + `/lakom/verify-cosign-signature
  failurePolicy: ` + strFailurePolicy + `
  matchPolicy: Equivalent
  name: verify-signature.lakom.service.extensions.gardener.cloud
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - kube-system
  objectSelector:
    matchExpressions:
    - key: resources.gardener.cloud/managed-by
      operator: In
      values:
      - gardener
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - v1
    operations:
    - CREATE
    - UPDATE
    resources:
    - pods
    - pods/ephemeralcontainers
  sideEffects: None
  timeoutSeconds: 25
`
}

func expectedShootRole() string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: gardener-extension-shoot-lakom-service-resource-reader
  namespace: kube-system
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
`
}

func expectedShootRoleBinding(saName string) string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: gardener-extension-shoot-lakom-service-resource-reader
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: gardener-extension-shoot-lakom-service-resource-reader
subjects:
- kind: ServiceAccount
  name: ` + saName + `
  namespace: kube-system
`
}

func expectedSeedConfigMap(namespace string) string {
	return `apiVersion: v1
data:
  scrape_config: |
    - job_name: extension-shoot-lakom-service
      honor_labels: false
      kubernetes_sd_configs:
      - role: endpoints
        namespaces:
          names: [` + namespace + `]
      relabel_configs:
      - source_labels:
        - __meta_kubernetes_service_name
        - __meta_kubernetes_endpoint_port_name
        action: keep
        regex: extension-shoot-lakom-service;metrics
      # common metrics
      - action: drop
        regex: __meta_kubernetes_service_label_(.+)
      - source_labels: [ __meta_kubernetes_pod_name ]
        target_label: pod
      - source_labels: [ __meta_kubernetes_pod_container_name ]
        target_label: container
      metric_relabel_configs:
      - source_labels: [ __name__ ]
        regex: ^lakom.*$
        action: keep
kind: ConfigMap
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
    extensions.gardener.cloud/configuration: monitoring
  name: extension-shoot-lakom-service-monitoring
  namespace: ` + namespace + `
`
}

func expectedSeedDeployment(replicas int32, namespace, genericKubeconfigSecretName, shootAccessSecretName, image, cosignPublicKeysSecretName, serverTLSSecretName string) string {
	var (
		genericKubeconfigSecretNameAnnotationKey = references.AnnotationKey("secret", genericKubeconfigSecretName)
		shootAccessSecretNameAnnotationKey       = references.AnnotationKey("secret", shootAccessSecretName)
		serverTLSSecretNameAnnotationKey         = references.AnnotationKey("secret", serverTLSSecretName)
		cosignPublicKeysSecretNameAnnotationKey  = references.AnnotationKey("secret", cosignPublicKeysSecretName)

		annotations = []string{
			genericKubeconfigSecretNameAnnotationKey + ": " + genericKubeconfigSecretName,
			shootAccessSecretNameAnnotationKey + ": " + shootAccessSecretName,
			serverTLSSecretNameAnnotationKey + ": " + serverTLSSecretName,
			cosignPublicKeysSecretNameAnnotationKey + ": " + cosignPublicKeysSecretName,
		}
	)

	return `apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    ` + strings.Join(annotations, "\n    ") + `
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
    high-availability-config.resources.gardener.cloud/type: server
  name: extension-shoot-lakom-service
  namespace: ` + namespace + `
spec:
  replicas: ` + fmt.Sprintf("%d", replicas) + `
  revisionHistoryLimit: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: lakom
      app.kubernetes.io/part-of: shoot-lakom-service
  strategy:
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
    type: RollingUpdate
  template:
    metadata:
      annotations:
        ` + strings.Join(annotations, "\n        ") + `
      creationTimestamp: null
      labels:
        app.kubernetes.io/name: lakom
        app.kubernetes.io/part-of: shoot-lakom-service
        networking.gardener.cloud/to-dns: allowed
        networking.gardener.cloud/to-private-networks: allowed
        networking.gardener.cloud/to-public-networks: allowed
        networking.resources.gardener.cloud/to-kube-apiserver-tcp-443: allowed
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: lakom
                  app.kubernetes.io/part-of: shoot-lakom-service
              topologyKey: kubernetes.io/hostname
            weight: 100
      automountServiceAccountToken: false
      containers:
      - args:
        - --cache-ttl=10m0s
        - --cache-refresh-interval=30s
        - --cosign-public-key-path=/etc/lakom/cosign/cosign.pub
        - --tls-cert-dir=/etc/lakom/tls
        - --health-bind-address=:8081
        - --metrics-bind-address=:8080
        - --port=10250
        - --kubeconfig=/var/run/secrets/gardener.cloud/shoot/generic-kubeconfig/kubeconfig
        image: ` + image + `
        imagePullPolicy: IfNotPresent
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 10
        name: lakom
        ports:
        - containerPort: 10250
          name: https
          protocol: TCP
        - containerPort: 8080
          name: metrics
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 5
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
        volumeMounts:
        - mountPath: /etc/lakom/cosign
          name: lakom-public-keys
          readOnly: true
        - mountPath: /etc/lakom/tls
          name: lakom-server-tls
          readOnly: true
        - mountPath: /var/run/secrets/gardener.cloud/shoot/generic-kubeconfig
          name: kubeconfig
          readOnly: true
      priorityClassName: gardener-system-300
      serviceAccountName: extension-shoot-lakom-service
      volumes:
      - name: lakom-public-keys
        secret:
          secretName: ` + cosignPublicKeysSecretName + `
      - name: lakom-server-tls
        secret:
          secretName: ` + serverTLSSecretName + `
      - name: kubeconfig
        projected:
          defaultMode: 420
          sources:
          - secret:
              items:
              - key: kubeconfig
                path: kubeconfig
              name: ` + genericKubeconfigSecretName + `
              optional: false
          - secret:
              items:
              - key: token
                path: token
              name: ` + shootAccessSecretName + `
              optional: false
status: {}
`
}

func expectedSeedPDB(namespace string) string {

	return `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: extension-shoot-lakom-service
  namespace: ` + namespace + `
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: lakom
      app.kubernetes.io/part-of: shoot-lakom-service
status:
  currentHealthy: 0
  desiredHealthy: 0
  disruptionsAllowed: 0
  expectedPods: 0
`
}

func expectedSeedSecretCosign(namespace, cosignSecretName string, cosignPublicKeys []string) string {
	indentedKeys := []string{}
	for _, key := range cosignPublicKeys {
		indentedKeys = append(indentedKeys, "    "+strings.TrimSuffix(strings.ReplaceAll(key, "\n", "\n    "), "    "))
	}

	return `apiVersion: v1
immutable: true
kind: Secret
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
    resources.gardener.cloud/garbage-collectable-reference: "true"
  name: ` + cosignSecretName + `
  namespace: ` + namespace + `
stringData:
  cosign.pub: |
` + strings.TrimSuffix(strings.Join(indentedKeys, "\n"), "\n") + `
type: Opaque
`
}

func expectedSeedService(namespace string) string {
	return `apiVersion: v1
kind: Service
metadata:
  annotations:
    networking.resources.gardener.cloud/from-policy-allowed-ports: '[{"protocol":"TCP","port":8080}]'
    networking.resources.gardener.cloud/from-policy-pod-label-selector: all-scrape-targets
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: extension-shoot-lakom-service
  namespace: ` + namespace + `
spec:
  ports:
  - name: https
    port: 443
    protocol: TCP
    targetPort: 10250
  - name: metrics
    port: 2718
    protocol: TCP
    targetPort: 8080
  selector:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  type: ClusterIP
status:
  loadBalancer: {}
`
}

func expectedSeedServiceAccount(namespace, serviceAccountName string) string {
	return `apiVersion: v1
automountServiceAccountToken: false
kind: ServiceAccount
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: ` + serviceAccountName + `
  namespace: ` + namespace + `
`
}

func expectedSeedVPA(namespace string) string {
	return `apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  creationTimestamp: null
  labels:
    app.kubernetes.io/name: lakom
    app.kubernetes.io/part-of: shoot-lakom-service
  name: extension-shoot-lakom-service
  namespace: ` + namespace + `
spec:
  resourcePolicy:
    containerPolicies:
    - containerName: lakom
      minAllowed:
        memory: 32Mi
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: extension-shoot-lakom-service
  updatePolicy:
    updateMode: Auto
status: {}
`
}
