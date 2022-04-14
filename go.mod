module github.com/chaosblade-io/chaosblade-operator

require (
	github.com/chaosblade-io/chaosblade-exec-cri v1.5.1-0.20220414062646-d65e7db648f7
	github.com/chaosblade-io/chaosblade-exec-os v1.5.1-0.20220414062130-f930629babd8
	github.com/chaosblade-io/chaosblade-spec-go v1.5.1-0.20220403044331-978f0910eaf3
	github.com/ethercflow/hookfs v0.3.0
	github.com/go-logr/logr v0.2.1 // indirect
	github.com/go-logr/zapr v0.2.0 // indirect
	github.com/go-openapi/spec v0.19.4
	github.com/hanwen/go-fuse v1.0.0
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.20.6
	k8s.io/apimachinery v0.20.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	sigs.k8s.io/controller-runtime v0.6.0
)

replace k8s.io/client-go => k8s.io/client-go v0.20.6 // Required by prometheus-operator

go 1.13
