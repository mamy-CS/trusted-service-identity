package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/kubernetes/pkg/apis/core/v1"

    "crypto/rand"

	ccorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
    "k8s.io/client-go/rest"
)


func pseudo_uuid()  (string, error) {
    b := make([]byte, 16)
    _, err := rand.Read(b)
    if err != nil {
        fmt.Println("Error: ", err)
        return "", err
    }

    uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

    return uuid, nil
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	// (https://github.com/kubernetes/kubernetes/issues/57982)
	defaulter = runtime.ObjectDefaulter(runtimeScheme)
)

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

const (
	admissionWebhookAnnotationInjectKey = "sidecar-injector-webhook.morven.me/inject"
	admissionWebhookAnnotationStatusKey = "sidecar-injector-webhook.morven.me/status"
)

type WebhookServer struct {
	initcontainerConfig *InitContainerConfig
	server              *http.Server
}

//initContainerConfig *InitContainerConfig
// Webhook Server parameters
type WhSvrParameters struct {
	port           int    // webhook server port
	certFile       string // path to the x509 certificate for https
	keyFile        string // path to the x509 private key matching `CertFile`
	initcontainerCfgFile string // path to initContainer injector configuration file
}

type InitContainerConfig struct {
    InitContainers  []corev1.Container   `yaml:"initContainers"`
	Volumes         []corev1.Volume      `yaml:"volumes"`
	AddVolumeMounts []corev1.VolumeMount `yaml:"addVolumeMounts"`
}

func (ic *InitContainerConfig) DeepCopy () *InitContainerConfig {
    icc := &InitContainerConfig{
        InitContainers: make([]corev1.Container, len(ic.InitContainers)),
        Volumes: make([]corev1.Volume, len(ic.Volumes)),
        AddVolumeMounts: make([]corev1.VolumeMount, len(ic.AddVolumeMounts)),
    }

    for i, v := range ic.InitContainers {
        icc.InitContainers[i] = *v.DeepCopy()
    }

    for i, v := range ic.Volumes {
        icc.Volumes[i] = *v.DeepCopy()
    }

    for i, v := range ic.AddVolumeMounts{
        icc.AddVolumeMounts[i] = *v.DeepCopy()
    }

    return icc
}




type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	_ = v1.AddToScheme(runtimeScheme)
}

// (https://github.com/kubernetes/kubernetes/issues/57982)
func applyDefaultsWorkaround(containers []corev1.Container, volumes []corev1.Volume) {
	defaulter.Default(&corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: containers,
			Volumes:    volumes,
		},
	})
}

func loadInitContainerConfig(configFile string) (*InitContainerConfig, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	glog.Infof("New configuration: sha256sum %x", sha256.Sum256(data))

	var cfg InitContainerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}


// Check whether the target resoured need to be mutated
func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta) bool {
	// skip special kubernete system namespaces
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %v for it' in special namespace:%v", metadata.Name, metadata.Namespace)
			return false
		}
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	status := annotations[admissionWebhookAnnotationStatusKey]

	// determine whether to perform mutation based on annotation for the target resource
	var required bool
	if strings.ToLower(status) == "injected" {
		required = false
	} else {
		switch strings.ToLower(annotations[admissionWebhookAnnotationInjectKey]) {
		default:
			required = false
		case "y", "yes", "true", "on":
			required = true
		}
	}

	glog.Infof("Mutation policy for %v/%v: status: %q required:%v", metadata.Namespace, metadata.Name, status, required)
	return required
}

func addContainer(target, added []corev1.Container, basePath string) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Container{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addVolume(target, added []corev1.Volume, basePath string) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Volume{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addVolumeMount(target, added []corev1.VolumeMount, basePath string) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.VolumeMount{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func updateAnnotation(target map[string]string, added map[string]string) (patch []patchOperation) {
	for key, value := range added {
		if target == nil || target[key] == "" {
			target = map[string]string{}
			patch = append(patch, patchOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					key: value,
				},
			})
		} else {
			patch = append(patch, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + key,
				Value: value,
			})
		}
	}
	return patch
}

// create mutation patch for resoures
func createPatch(pod *corev1.Pod, initcontainerConfig *InitContainerConfig, annotations map[string]string) ([]byte, error) {
	var patch []patchOperation

    // additions
    glog.Infof("add volumes: %v", initcontainerConfig.Volumes)
	patch = append(patch, updateAnnotation(pod.Annotations, annotations)...)
    patch = append(patch, addContainer(pod.Spec.InitContainers, initcontainerConfig.InitContainers, "/spec/initContainers")...)
	patch = append(patch, addVolume(pod.Spec.Volumes, initcontainerConfig.Volumes, "/spec/volumes")...)

    for i, c := range pod.Spec.Containers {
        glog.Infof("add vol mounts : %v", addVolumeMount(c.VolumeMounts, initcontainerConfig.AddVolumeMounts, fmt.Sprintf("/spec/containers/%d/volumeMounts", i)))
        patch = append(patch, addVolumeMount(c.VolumeMounts, initcontainerConfig.AddVolumeMounts, fmt.Sprintf("/spec/containers/%d/volumeMounts", i))...)
    }

    
	return json.Marshal(patch)
}

// main mutation process
func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request

    namespace := req.Namespace
    if namespace == metav1.NamespaceNone {
        namespace = metav1.NamespaceDefault
    }
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, pod.Name, req.UID, req.Operation, req.UserInfo)

	// determine whether to perform mutation
	if !mutationRequired(ignoredNamespaces, &pod.ObjectMeta) {
		glog.Infof("Skipping mutation for %s/%s due to policy check", pod.Namespace, pod.Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}


    initcontainerConfigCp := whsvr.initcontainerConfig.DeepCopy()
    
    

    glog.Infof("Applying defaults")

	// Workaround: https://github.com/kubernetes/kubernetes/issues/57982
	applyDefaultsWorkaround(initcontainerConfigCp.InitContainers, initcontainerConfigCp.Volumes)

    var err error

    // XXX: Added workaround for //github.com/kubernetes/kubernetes/issues/57982
    // for service accounts
    if len(pod.Spec.Containers) == 0 {
        err =  fmt.Errorf("Pod has no containers")
		glog.Infof("Err: %v", err)
        return &v1beta1.AdmissionResponse{
            Result: &metav1.Status{
                Message: err.Error(),
            },
        }
    }
    var serviceaccountVolMount corev1.VolumeMount
    foundServiceAccount := false
    for _, vmount := range pod.Spec.Containers[0].VolumeMounts {
        if strings.Contains(vmount.Name , "token") {
            serviceaccountVolMount = vmount
            foundServiceAccount = true
            break
        }
    }

    if !foundServiceAccount {
        err =  fmt.Errorf("service account token not found")
		glog.Infof("Err: %v", err)
        return &v1beta1.AdmissionResponse{
            Result: &metav1.Status{
                Message: err.Error(),
            },
        }
    }

    for i, c := range initcontainerConfigCp.InitContainers {
        glog.Infof("add vol mounts (initc) : %v", c.VolumeMounts, serviceaccountVolMount)
        initcontainerConfigCp.InitContainers[i].VolumeMounts = append(c.VolumeMounts, serviceaccountVolMount)
    }

    for i:=0; i < len(pod.Spec.Containers); i++ {

    }

    // Set volume 
    uuid, err := pseudo_uuid()
    if err != nil {
        glog.Infof("Err: %v", err)
        return &v1beta1.AdmissionResponse{
            Result: &metav1.Status{
                Message: err.Error(),
            },
        }
    }

    secretName := "ti-secret-" + uuid
    
    // Create a secret
    glog.Infof("Getting cluster Config")
    kubeConf, err := rest.InClusterConfig()
	if err != nil {
		glog.Infof("Err: %v", err)
		return &v1beta1.AdmissionResponse{
            Result: &metav1.Status{
                Message: err.Error(),
            },
        }
	}

    glog.Infof("Creating kube client")
	client, err := ccorev1.NewForConfig(kubeConf)
	if err != nil {
		glog.Infof("Err: %v", err)
		return &v1beta1.AdmissionResponse{
            Result: &metav1.Status{
                Message: err.Error(),
            },
        }
	}


    // Create secret if it doesn't exist
    glog.Infof("Creating secret")
    createSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName}}
    createSecret, err = client.Secrets(namespace).Create(createSecret)
    if err != nil {
        if !errors.IsAlreadyExists(err) {
            glog.Infof("Err: %v", err)
            return &v1beta1.AdmissionResponse{
                Result: &metav1.Status{
                Message: err.Error(),
                },
            }
        }
    }

    glog.Infof("pod name : %v", pod.ObjectMeta.Name)
    for i,v := range initcontainerConfigCp.Volumes {
        if v.Name == "ti-vault-secret" {
            initcontainerConfigCp.Volumes[i].VolumeSource.Secret.SecretName = secretName
            break
        }
    }

    glog.Infof("add vol  : %v", initcontainerConfigCp.Volumes)
    annotations := map[string]string{admissionWebhookAnnotationStatusKey: "injected", "ti-secret-key" : secretName}

    // Create TI secret key to populate
    glog.Infof("Creating patch")
	patchBytes, err := createPatch(&pod, initcontainerConfigCp, annotations)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// Serve method for webhook server
func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = whsvr.mutate(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	glog.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
