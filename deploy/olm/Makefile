.PHONY: build clean

BLADE_VERSION=0.6.0

# Build [OLM](https://github.com/operator-framework/operator-lifecycle-manager)
build:
	operator-sdk olm-catalog gen-csv \
	  --operator-name chaosblade-operator \
	  --csv-version $(BLADE_VERSION) \
	  --update-crds --verbose
	# Change `olm` keyword to `chaosblade-operator`
	sed 's/olm/chaosblade-operator/g' deploy/olm-catalog/olm/olm.package.yaml \
      > deploy/olm-catalog/chaosblade-operator/chaosblade-operator.package.yaml

	rm -rf deploy/olm-catalog/olm

clean:
	rm -rf deploy/olm-catalog