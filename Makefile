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
#
# Built from the sibling repository ../indecis. See docs/model.md.

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


.PHONY: mail build test backbone data model model-a25m eval serve clean

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

clean:
	rm -rf bin $(BUILD)
