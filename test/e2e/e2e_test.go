/*
Copyright 2025.

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

package e2e

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotclient "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/outscale/csi-snapshot-exporter/internal/controller"
	"github.com/outscale/csi-snapshot-exporter/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
	admissionapi "k8s.io/pod-security-admission/api"
)

// namespace where the project is deployed in
const namespace = "csi-snapshot-exporter-system"

// serviceAccountName created for the project
const serviceAccountName = "csi-snapshot-exporter-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "csi-snapshot-exporter-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "csi-snapshot-exporter-metrics-binding"

func init() {
	if os.Getenv("KUBECONFIG") == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		_ = os.Setenv("KUBECONFIG", kubeconfig)
	}
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.TestContext.KubeConfig = os.Getenv("KUBECONFIG")
	testing.Init()
	flag.Parse()
	framework.AfterReadingAllFlags(&framework.TestContext)
	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(20 * time.Second)
}

//nolint:lll
var _ = Describe("Snapshot exporter", Ordered, func() {
	var (
		controllerPodName string
	)

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("deploy snapshotter CRDs")
		cmd = exec.Command("kubectl", "apply", "-f",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
		cmd = exec.Command("kubectl", "apply", "-f",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
		cmd = exec.Command("kubectl", "apply", "-f",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
		cmd = exec.Command("kubectl", "apply", "-f",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
		cmd = exec.Command("kubectl", "apply", "-f",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("deploying the CSI secret")
		cmd = exec.Command("kubectl", "create", "secret", "generic", "osc-csi-bsu",
			"--from-literal=access_key="+os.Getenv("OSC_ACCESS_KEY"), "--from-literal=secret_key="+os.Getenv("OSC_SECRET_KEY"),
			"-n", "kube-system")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the CSI secret")

		By("deploying the CSI driver")
		cmd = exec.Command("helm", "upgrade", "--install", "osc-bsu-csi-driver", "oci://docker.io/outscalehelm/osc-bsu-csi-driver",
			"--namespace", "kube-system", "--set", "enableVolumeSnapshot=true", "--set", "region="+os.Getenv("OSC_REGION"), "--set", "verbose=5")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the CSI driver")

		By("deploying the exporter secret")
		cmd = exec.Command("kubectl", "create", "secret", "generic", "osc-csi-bsu",
			"--from-literal=access_key="+os.Getenv("OSC_ACCESS_KEY"), "--from-literal=secret_key="+os.Getenv("OSC_SECRET_KEY"), "--from-literal=region="+os.Getenv("OSC_REGION"),
			"-n", namespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the exporter secret")

		By("deploying osc-snapshot-exporter")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy osc-snapshot-exporter")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying osc-snapshot-exporter")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching snapshots")
			cmd := exec.Command("kubectl", "get", "volumesnapshots,volumesnapshotcontents", "-A")
			snapshots, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Snapshots:\n %s", snapshots)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get snapshots: %s", err)
			}
			By("Fetching controller manager pod logs")
			cmd = exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	Context("Snapshot exporter controller", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=csi-snapshot-exporter-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})
	Context("Snapshot exports", func() {
		var (
			ns  string
			cs  kubernetes.Interface
			scs snapshotclient.Interface
			ctx context.Context
			pvc *corev1.PersistentVolumeClaim
		)

		f := framework.NewDefaultFramework("snapshot-exporter")
		f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
		bucket := os.Getenv("EXPORT_BUCKET")

		BeforeEach(func() {
			ctx = context.TODO()
			cs = f.ClientSet
			scs = utils.SnapshotClient()
			ns = f.Namespace.Name
			By("Creating storage class")
			utils.CreateStorageClass(ctx, cs)
			By("Creating PVC")
			pvc = utils.CreatePVC(ctx, cs, ns)
		})
		AfterEach(func() {
			By("cleaning up")
			cmd := exec.Command("kubectl", "delete", "ns", ns)
			_, _ = utils.Run(cmd)
		})

		It("should export a snapshot in qcow2 format", func() {
			snapshotClass := f.UniqueName
			By("Creating volume snapshot class")
			utils.CreateSnapshotClass(ctx, scs, snapshotClass, map[string]string{
				controller.ParamExportEnabled: "true",
				controller.ParamExportFormat:  "qcow2",
				controller.ParamExportBucket:  bucket,
				controller.ParamExportPrefix:  "snapshot-exporter/{date}/{ns}/{vs}/",
			},
			)
			By("Creating snapshot")
			snap := &snapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "snap-",
					Namespace:    pvc.Namespace,
				},
				Spec: snapshotv1.VolumeSnapshotSpec{
					Source: snapshotv1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvc.Name,
					},
					VolumeSnapshotClassName: &snapshotClass,
				},
			}
			snap, err := scs.SnapshotV1().VolumeSnapshots(pvc.Namespace).Create(ctx, snap, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			By("Waiting for snapshot content")
			Eventually(func() *snapshotv1.VolumeSnapshotStatus {
				snap, err = scs.SnapshotV1().VolumeSnapshots(pvc.Namespace).Get(ctx, snap.Name, metav1.GetOptions{})
				framework.ExpectNoError(err)
				return snap.Status
			}, 10*time.Minute).Should(HaveField("BoundVolumeSnapshotContentName", Not(BeNil())))
			By("Waiting for export id and state")
			Eventually(func() metav1.ObjectMeta {
				snapc, err := scs.SnapshotV1().VolumeSnapshotContents().Get(ctx, *snap.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
				framework.ExpectNoError(err)
				return snapc.ObjectMeta
			}, 10*time.Minute).Should(HaveField("Annotations", And(
				HaveKey(controller.AnnotationExportTask),
				HaveKey(controller.AnnotationExportState),
			)))
			By("Waiting for completed export")
			Eventually(func() metav1.ObjectMeta {
				snapc, err := scs.SnapshotV1().VolumeSnapshotContents().Get(ctx, *snap.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
				framework.ExpectNoError(err)
				_, _ = fmt.Fprintf(GinkgoWriter, "status: %q\n", snapc.Annotations[controller.AnnotationExportState])
				return snapc.ObjectMeta
			}, 30*time.Minute).Should(HaveField("Annotations", And(
				HaveKeyWithValue(controller.AnnotationExportState, "completed"),
				HaveKeyWithValue(controller.AnnotationExportPath, And(
					ContainSubstring(snap.Name),
					ContainSubstring(snap.Namespace),
				))),
			))
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
