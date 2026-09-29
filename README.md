# indecis-email-triage

`indecis-mail` sorts the emails of an IMAP mailbox among categories you describe in a YAML file, with a few example emails each. It tags them with IMAP keywords, or copies or moves them to folders. The model runs on your machine, on CPU, with [indecis](https://github.com/Bornholm/indecis): no email leaves it.

```bash
make mail backbone          # bin/indecis-mail, and bekko-embedding-v1-a8m (210 MB)
cp config.example.yaml mail.yaml
export IMAP_USER=me@example.org IMAP_PASSWORD='...'
bin/indecis-mail check -config mail.yaml              # connection, folders, keywords
bin/indecis-mail run -config mail.yaml -dry-run       # what would be done
bin/indecis-mail run -config mail.yaml                # do it
bin/indecis-mail run -config mail.yaml -watch 5m      # again every 5 minutes
```

## Configuration

`config.example.yaml` documents every field. In short:

- `imap`: address, security (`tls`, `starttls` or `none`), username and password. Values can come from the environment: `${VAR}`, `${VAR:-default}`, `${VAR:?message}` to stop when a variable is missing.
- `model`: the backbone as downloaded, or the model fine-tuned on emails (`make model`, see [docs/model.md](docs/model.md)).
- `select`: only unread messages, since a date or a duration, at most so many.
- `action`: `keyword` tags the message in place (`triage/facturation-et-paiement`), `copy` and `move` send it to `Triage/<category>`. Under `threshold`, a message is left as is.
- `categories`: a name, a description and example emails. The examples matter most: with the backbone as downloaded, 5 examples per category took the share of emails sorted right on the imnim test set from 69% to 92% in the proof of concept below.

Messages are read without being marked as read. A state file (`mail.state.json` next to the configuration) remembers the messages already sorted: each is sorted once, whatever the action.

## Tuning

`bin/indecis-mail classify -config mail.yaml *.eml` sorts saved emails without touching the mailbox, with the confidence of each. Save a few dozen of your emails, check the sort, add examples to the categories that go wrong, and set `threshold` where the mistakes are: the probabilities are not calibrated.

Some servers, Gmail among them, do not keep arbitrary keywords: `check` says so, and `copy` or `move` then works.

## The experiments behind the model

The proof of concept that measured this approach is in `experiments/`: data preparation, fine-tuning and evaluation on support tickets, Enron emails labeled by LLMs, French translations and French emails produced by templates.

### Reproducing the model

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

### Documentation

- [docs/model.md](docs/model.md): reproducing the models, results.
- [docs/data.md](docs/data.md): sources, licenses, labeling by LLMs.

## License

GPL-3.0. The support tickets used for training are under CC-BY-NC-4.0: a model trained on them is fit for experiments only.
