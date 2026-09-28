# Data

## Downloaded (`make data`)

| File | Source | License | Use |
| --- | --- | --- | --- |
| `tickets.jsonl` | [Tobi-Bueck/customer-support-tickets](https://huggingface.co/datasets/Tobi-Bueck/customer-support-tickets) | CC-BY-NC-4.0 | training, and test on unseen queues |
| `imnim.jsonl` | [imnim/multiclass-email-classification](https://huggingface.co/datasets/imnim/multiclass-email-classification) | MIT | test only |
| `enron.jsonl` | [corbt/enron-emails](https://huggingface.co/datasets/corbt/enron-emails) | Enron corpus, released by FERC | unlabeled pool |
| `asn.jsonl` | [AdrienB134/ASN_Lettres_De_Suivi](https://huggingface.co/datasets/AdrienB134/ASN_Lettres_De_Suivi) | see the dataset page | test only, French |

The files are fetched through the Hugging Face datasets server. The tickets are never versioned here because of their non-commercial license.

## Versioned (`data/`)

LLM answers cannot be reproduced exactly, so they are versioned:

| File | Emails | Content |
| --- | --- | --- |
| `enron_labeled.jsonl` | 430 | Enron test emails, classic list of 13 categories, labeled by teacher consensus |
| `enron_labeled_fr.jsonl` | 410 | the same emails translated into French |
| `enron_train.jsonl` | 1,934 | Enron training emails, labeled along eight other category lists |
| `enron_train_fr.jsonl` | 850 | French translations of part of them |

The inputs given to the teachers are next to them: `classic_schema.json` and `classic_guidelines.md` (test list), `train_schema.json` and `train_guidelines.md` (training lists, written by `go run . teacher-schema`), `translate_fr.txt` (translation instructions).

The teachers are the same as for indecis (Claude and Pi, through `indecis-teach` from the indecis `teacher` module). Only the emails where they agree are kept.

## Templates (`templates/`)

20 French email scenarios (invoices, meetings, leave, outages, formal notices…), with greetings and sign-offs in three registers, rendered by `indecis synth`. Their labels follow five axes (department, intent, sender, deadline, register), whose category lists are in `taxonomies.go`.
