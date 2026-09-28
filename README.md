# indecis-email-triage

Classifies emails among categories chosen at call time, with an [indecis](../indecis) model. The categories are not fixed in the model: each request lists them, by name, by a one-line description, or by a few example emails.

The model is a small encoder fine-tuned on support tickets, Enron emails labeled by LLMs, their French translations and French emails produced by templates. It runs on CPU in pure Go, in about 13 ms per email on a8m.

This is a proof of concept: the test sets were not reviewed by hand, and each result comes from a single training run.

## Getting started

```bash
make data     # versioned LLM labels, plus public datasets downloaded
make model    # about 45 min on a laptop, evaluation included
make serve    # indecis-serve on 127.0.0.1:8080, model name "emails"
```

Then ask for a category:

```bash
curl -s localhost:8080/api/alpha/decisions -d '{
  "model": "emails",
  "state": "Subject: Invoice 2024-118 reminder\n\nHello, the invoice of March 12 is still unpaid.",
  "questions": {"category": {"type": "choice", "instructions": "Email category",
    "criteria": {"Technical support": "Outage, bug, access problem",
                 "Billing and payment": "Invoices, payments, reminders, refunds",
                 "Human resources": "Hiring, leave, payroll"}}}
}' | jq -r .answers.category.choice
```

The request format, examples in criteria and the server options are described in [indecis docs/serving.md](../indecis/docs/serving.md) and [docs/open-categories.md](../indecis/docs/open-categories.md).

## Documentation

- [docs/model.md](docs/model.md): reproducing the models, results.
- [docs/data.md](docs/data.md): sources, licenses, labeling by LLMs.

## License

GPL-3.0, like indecis. The support tickets used for training are under CC-BY-NC-4.0: a model trained on them is fit for experiments only.
