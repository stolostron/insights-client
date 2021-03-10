package retriever

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-cluster-management/insights-client/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPullSecretExists(t *testing.T) {
	pullSecret, err := config.GetKubeClient().CoreV1().Secrets(OpenShiftConfig).Get(context.Background(), PullSecret, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Fatalf("The pull-secret should exist when cluster boots up: %s", err)
	}
	if err != nil {
		t.Fatalf("The pull-secret read failed: %s", err)
	}
	var (
		secretConfig []byte
		ok           bool
	)
	if secretConfig, ok = pullSecret.Data[".dockerconfigjson"]; !ok {
		t.Fatalf("The pull-secret didn't contain .dockerconfigjson key: %s", err)
	}
	obj := map[string]interface{}{}
	errUnmarshal := json.Unmarshal(secretConfig, &obj)
	if errUnmarshal != nil {
		t.Fatal(errUnmarshal.Error())
	}
	creds := obj["auths"].(map[string]interface{})
	if _, ok := creds[CloudOpenShiftCom]; !ok {
		t.Fatalf("not found secret for cloud.openshift.com")
	}
}
