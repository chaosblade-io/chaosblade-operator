module github.com/chaosblade-io/chaosblade-operator

require (
	github.com/chaosblade-io/chaosblade-exec-docker v0.6.1-0.20200916065009-654d2ecba736
	github.com/chaosblade-io/chaosblade-exec-os v0.6.1-0.20200921042733-687f06558524
	github.com/chaosblade-io/chaosblade-spec-go v0.6.1-0.20200917030038-f8260e80f639
	github.com/ethercflow/hookfs v0.3.0
	github.com/go-openapi/spec v0.19.4
	github.com/hanwen/go-fuse v1.0.0
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-runtime v0.5.3
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)

go 1.13
