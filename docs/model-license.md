# License of this model

This model is for non-commercial use only.

It is [bekko-embedding-v1-a8m](https://huggingface.co/hotchpotch/bekko-embedding-v1-a8m) (MIT license) fine-tuned by [indecis-email-triage](https://github.com/Bornholm/indecis-email-triage) with `make model`. Its training data include:

| Data | Source | License |
| --- | --- | --- |
| 3,000 support tickets | [Tobi-Bueck/customer-support-tickets](https://huggingface.co/datasets/Tobi-Bueck/customer-support-tickets) | [CC-BY-NC-4.0](https://creativecommons.org/licenses/by-nc/4.0/) |
| 1,934 Enron emails, labeled by LLMs, and 850 French translations | [corbt/enron-emails](https://huggingface.co/datasets/corbt/enron-emails), labels and translations in the repository's `data/` | Enron corpus, released by FERC |
| 3,000 French emails rendered from templates | the repository's `experiments/templates/` | GPL-3.0 |

Because the weights were trained on the support tickets, they are an adaptation of a CC-BY-NC-4.0 work: you may share and adapt them for non-commercial purposes, giving credit to the datasets above and stating your changes. The weights were changed by fine-tuning; the tickets themselves are not included.

The code that trains and runs the model is under GPL-3.0. See [docs/model.md](https://github.com/Bornholm/indecis-email-triage/blob/main/docs/model.md) for the training and the measured accuracy.
