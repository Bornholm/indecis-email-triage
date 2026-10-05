# Email sorting with indecis: the indecis-mail tool, and the experiments
# behind its model.
#
#   make mail        builds bin/indecis-mail
#   make test        tests the tool (an in-memory IMAP server)
#
#   make data        versioned LLM labels + public datasets downloaded
#   make model       a8m, tickets + Enron + French translations + templates
#   make model-a25m  a25m, tickets + Enron (best measured model)
#   make eval        backbone and fine-tuned model on the test sets
#   make serve       serves build/model with indecis-serve
#   make release-model  attaches build/model to the latest release
#                       (MODEL_TAG=v1.2.3 for another one)
#
# The programs depend on indecis v0.3.0; make serve builds indecis-serve from
# the sibling repository ../indecis. See docs/model.md.

GOEXPERIMENT ?= simd
export GOEXPERIMENT

INDECIS_SRC ?= ../indecis
MODELS ?= $(HOME)/.cache/indecis/models
BACKBONE ?= $(MODELS)/bekko-embedding-v1-a8m
BACKBONE_A25M ?= $(MODELS)/bekko-embedding-v1-a25m
BUILD ?= build
DATA := $(BUILD)/data
MODEL ?= $(BUILD)/model
RUN := go run ./experiments
# Latest published release, only looked up by release-model.
MODEL_TAG ?= $(shell gh release view --json tagName -q .tagName 2>/dev/null)
MODEL_ASSET = indecis-email-triage-model_$(MODEL_TAG)


.PHONY: mail build test backbone data model model-a25m eval serve release-model clean

mail:
	CGO_ENABLED=0 go build -o bin/indecis-mail ./cmd/indecis-mail

build: mail
	go build -o bin/email-triage ./experiments

test:
	go vet ./...
	go test ./...

# Backbones: bekko-embedding-v1 a8m (210 MB) and a25m (MIT).
backbone: $(BACKBONE)/model.safetensors

$(MODELS)/%/model.safetensors:
	mkdir -p $(MODELS)/$*
	for f in config.json model.safetensors tokenizer.json; do \
		curl -sfL -o $(MODELS)/$*/$$f https://huggingface.co/hotchpotch/$*/resolve/main/$$f; \
	done

# LLM-labeled Enron emails and translations are versioned in data/; the
# public datasets are downloaded (see docs/data.md).
data:
	mkdir -p $(DATA)
	cp data/*.jsonl data/*.json data/*.md data/*.txt $(DATA)/
	$(RUN) prepare -data $(DATA)
	$(RUN) asn -data $(DATA)

model: backbone
	$(RUN) train-embed -data $(DATA) -backbone $(BACKBONE) -out $(MODEL) \
		-n 3000 -epochs 1 -fr -synth 3000

model-a25m: $(BACKBONE_A25M)/model.safetensors
	$(RUN) train-embed -data $(DATA) -backbone $(BACKBONE_A25M) -out $(MODEL)-a25m \
		-n 3000 -epochs 1

eval:
	$(RUN) eval-embed -data $(DATA) -backbone $(BACKBONE) -out $(MODEL)
	$(RUN) few-shot -data $(DATA) -backbone $(BACKBONE) -out $(MODEL) -k 5

serve:
	cd $(INDECIS_SRC)/decision && go build -o $(CURDIR)/bin/indecis-serve ./cmd/indecis-serve
	bin/indecis-serve -model emails=$(MODEL)

# The model is trained locally (make data model) and attached to a release
# made by the CI, with its non-commercial license (docs/model-license.md).
release-model:
	@test -n "$(MODEL_TAG)" || { echo "no release found, set MODEL_TAG"; exit 1; }
	@test -s $(MODEL)/model.safetensors || { echo "no model in $(MODEL), run make data model"; exit 1; }
	rm -rf $(BUILD)/$(MODEL_ASSET)
	mkdir -p $(BUILD)/$(MODEL_ASSET)
	cp -r $(MODEL)/. $(BUILD)/$(MODEL_ASSET)/
	cp docs/model-license.md $(BUILD)/$(MODEL_ASSET)/LICENSE.md
	tar -C $(BUILD) -czf $(BUILD)/$(MODEL_ASSET).tar.gz $(MODEL_ASSET)
	cd $(BUILD) && sha256sum $(MODEL_ASSET).tar.gz > $(MODEL_ASSET).tar.gz.sha256
	gh release upload $(MODEL_TAG) $(BUILD)/$(MODEL_ASSET).tar.gz $(BUILD)/$(MODEL_ASSET).tar.gz.sha256

clean:
	rm -rf bin $(BUILD)
