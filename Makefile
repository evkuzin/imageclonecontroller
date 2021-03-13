
DATE=$(shell date +%s)


docker:
	@docker build -t evkuzin/imageloader .
	@docker push evkuzin/imageloader
	@sed -e 's/REPLACEME/'\"$(DATE)\"'/' deployment-manifest.yaml | kubectl --context $(CONTEXT) apply -f -
	@kubectl --context $(CONTEXT) apply -f ./cm-manifest.yaml


k8s:
	@sed -e 's/REPLACEME/'\"$(DATE)\"'/' deployment-manifest.yaml | kubectl --context $(CONTEXT) apply -f -
	@kubectl --context $(CONTEXT) apply -f ./cm-manifest.yaml
