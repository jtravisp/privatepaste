IMAGE ?= privatepaste:dev
CLUSTER ?= privatepaste

.PHONY: help test vet run image kind-up kind-load kind-deploy kind-down

help:
	@echo "test        - go vet + unit tests (race detector needs cgo)"
	@echo "run         - run locally against DYNAMODB_ENDPOINT or AWS (reads .env)"
	@echo "image       - build the container image ($(IMAGE))"
	@echo "kind-up     - create a local kind cluster with Kyverno and the policies"
	@echo "kind-deploy - build, load and deploy the app + DynamoDB Local into kind"
	@echo "kind-down   - delete the local kind cluster"

vet:
	go vet ./...

test: vet
	go test -count=1 ./...

run:
	go run ./cmd/server

image:
	docker build -t $(IMAGE) .

kind-up:
	../scripts/kind-up.sh $(CLUSTER)

kind-deploy: image
	kind load docker-image $(IMAGE) --name $(CLUSTER)
	kubectl --context kind-$(CLUSTER) apply -k ../deploy/k8s/overlays/kind
	kubectl --context kind-$(CLUSTER) -n privatepaste rollout restart deployment/privatepaste
	kubectl --context kind-$(CLUSTER) -n privatepaste rollout status deployment/privatepaste --timeout=120s

kind-down:
	kind delete cluster --name $(CLUSTER)
