# The models

Both models are indecis encoders fine-tuned for open mode (`FitEmbeddings`): they learn to bring an email close to the description of its category and away from the others, over many different category lists. A category list never seen in training works the same way.

## Reproducing

```bash
make data          # build/data
make model         # a8m: build/model
make model-a25m    # a25m: build/model-a25m, three times slower
make eval          # backbone, fine-tuned model, and 5 examples per category
```

The programs depend on indecis v0.3.0 (see `go.mod`).

| Target | Backbone | Training data | Steps |
| --- | --- | --- | --- |
| `model` | a8m | 3,000 tickets, 1,934 labeled Enron emails, 850 French translations, 3,000 template emails | 2,517 |
| `model-a25m` | a25m | 3,000 tickets, 1,934 labeled Enron emails | 1,258 |

Each Enron email trains along eight category lists (`taxonomies.go`), never the "classic" list of the test. Training is one epoch, batches of 16, learning rate 2e-5, seed 1.

## Publishing

A tag `v*` pushed to GitHub makes the CI publish `indecis-mail` with goreleaser. The model is trained locally, from the tagged commit, and attached to the release afterwards:

```bash
make data model
make release-model                   # the latest release
make release-model MODEL_TAG=v0.1.0  # another one
```

`release-model` uploads `indecis-email-triage-model_<tag>.tar.gz` and its SHA-256 with `gh`. The archive holds `build/model` and [model-license.md](model-license.md) as `LICENSE.md`: the support tickets make the weights non-commercial.

## Results

Accuracy with the category name and description only, then with 5 example emails per category:

| Test set | Categories | Backbone a8m | `model` (a8m) | `model-a25m` | `model`, 5 examples | `model-a25m`, 5 examples |
| --- | --- | --- | --- | --- | --- | --- |
| imnim, English | 10 | 69.2% | 80.8% | 76.8% | 94.0% | 96.8% |
| Enron, classic list | 13 | 27.9% | 42.1% | 52.8% | 48.6% | 58.0% |
| Enron translated into French | 13 | 23.7% | 34.6% | | 50.3% | |
| ASN letters, French, topic | 12 | 21.8% | 24.4% | | 71.8% | |

Empty cells were not measured. Measured on September 27, 2026, one training run each; between runs, the standard deviation is about 2 points at these test sizes.

- **Examples help more than fine-tuning.** Five examples per category bring ASN from 24% to 72%; fine-tuning alone brings nothing on its specialized vocabulary.
- **French templates help imnim and French a little.** On a8m, adding the templates took imnim from 72% to 80%, the French Enron from 31% to 35%. The translations brought little.
- **a25m is about 10 points better on fine categories** (Enron), for three times the compute: 40 ms per email instead of 13.

## Limits

- The probabilities are not calibrated. To catch an email that fits no category, tune a threshold on a few dozen examples, or add an "Other" category described by examples.
- The models read 256 tokens: the subject and the start of the body. Remove long quotes before sending.
- The test labels come from LLMs (Enron) or from document metadata (ASN), not from a human review.
