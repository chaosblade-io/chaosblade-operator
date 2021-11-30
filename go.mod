module github.com/chaosblade-io/chaosblade-operator

require (
	github.com/chaosblade-io/chaosblade-exec-cri v0.0.0-20211125032821-6859ddfdf8de
	github.com/chaosblade-io/chaosblade-exec-docker v1.3.1-0.20210906073714-7bd7d7367d76
	github.com/chaosblade-io/chaosblade-exec-os v1.3.1-0.20210906070659-0b8e3c15c25b
	github.com/chaosblade-io/chaosblade-spec-go v1.3.1-0.20211124120331-a95ad0aac789
	github.com/ethercflow/hookfs v0.3.0
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

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.20.6 // Required by prometheus-operator
)

go 1.13
