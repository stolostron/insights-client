module github.com/stolostron/insights-client

go 1.18

require (
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/golang/glog v1.0.0
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kennygrant/sanitize v1.2.4
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20220121210141-e204ce36a2ba // indirect
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8 // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	k8s.io/api v0.23.2
	k8s.io/apimachinery v0.23.2
	k8s.io/client-go v0.23.2
	k8s.io/klog/v2 v2.40.1 // indirect
	k8s.io/utils v0.0.0-20211208161948-7d6a63dca704 // indirect
	open-cluster-management.io/api v0.6.0
	sigs.k8s.io/controller-runtime v0.11.0 // indirect
	sigs.k8s.io/wg-policy-prototypes v0.0.0-20211213184226-468085642d55
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/time v0.0.0-20211116232009-f0f3c7e86c11 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/kube-openapi v0.0.0-20220114203427-a0453230fd26 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
	golang.org/x/crypto v0.0.0-20211215165025-cf75a172585e // indirect - keep this in go.mod

)

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20211215165025-cf75a172585e
