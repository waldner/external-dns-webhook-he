# `external-dns` Webhook Provider for HE DNS

[![Go Report Card](https://goreportcard.com/badge/github.com/waldner/external-dns-webhook-he)](https://goreportcard.com/report/github.com/waldner/external-dns-webhook-he)
[![Releases](https://img.shields.io/github/v/tag/waldner/external-dns-webhook-he)](https://github.com/waldner/external-dns-webhook-he/tags)
[![LICENSE](https://img.shields.io/github/license/waldner/external-dns-webhook-he)](https://github.com/waldner/external-dns-webhook-he/blob/master/LICENSE)

A webhook to use [HE DNS](https://dns.he.net) as a webhook provider for [external-dns](https://github.com/kubernetes-sigs/external-dns).


## Installation

You must create a secret with your HE credentials, in the same namespace where externaldns is running. Example:

```
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: he-credentials
  namespace: external-dns
stringData:
  username: he_username
  password: he_password

```

The webhook runs as a sidecar container to the main exernaldns pod. Here is a sample `values.yaml` for use with Bitnami's externaldns chart, which uses the above secret to extract the values of environment variables to inject into the webhook container:

```
# special version of externalds with webhook support,
# waiting for a release
image:
  registry: docker.io
  repository: waldner/external-dns
  tag: v0.13.6-126-gd7cec324-dirty

provider: webhook
interval: 60m
triggerLoopOnEvent: true
logLevel: trace
txtPrefix: "zzz-"
policy: upsert-only   # or "sync" for full sync (ie also deletions)
txtOwnerId: blah

extraArgs:
  webhook-provider-url: http://localhost:3333

sidecars:
  - name: he-webhook
    image: ghcr.io/waldner/external-dns-webhook-he:0.0.2   # or whatever
    imagePullPolicy: IfNotPresent
    ports:
      - containerPort: 3333
        name: http
    livenessProbe:
      httpGet:
        path: /health
        port: http
      initialDelaySeconds: 10
      timeoutSeconds: 5
    readinessProbe:
      httpGet:
        path: /health
        port: http
      initialDelaySeconds: 10
      timeoutSeconds: 5
    env:
      - name: WEBHOOK_HE_LOG_LEVEL
        value: "info"
      - name: WEBHOOK_HE_USERNAME
        valueFrom:
          secretKeyRef:
            name: he-credentials
            key: username
      - name: WEBHOOK_HE_PASSWORD
        valueFrom:
          secretKeyRef:
            name: he-credentials
            key: password
      - name: WEBHOOK_HE_DOMAIN_FILTER
        value: "example.com"
```

To install, thus run:

```bash
helm upgrade --install -n external-dns --create-namespace external-dns bitnami/external-dns -f values.yaml
```

Check the logs with

```bash
kubectl logs -n external-dns -f "$(kubectl get pods -n external-dns | awk 'NR>1 && $1 ~ /external-dns/{print $1; exit}')" -c he-webhook
```

## Concepts and configuration

The webhook is configured using environment variables. Here's a list:

```
WEBHOOK_HE_USERNAME: mandatory
WEBHOOK_HE_PASSWORD: mandatory
WEBHOOK_HE_LOG_LEVEL: can be a string (eg "info", "debug" etc) or a numeric value (higher means more verbose). Default: info
WEBHOOK_HE_URL: default is "https://dns.he.net"

WEBHOOK_HE_DOMAIN_FILTER: a list of domains to watch, eg "foo.com,bar.com", can also be just one of course
WEBHOOK_HE_DOMAIN_FILTER_EXCLUDE: a list of domains to ignore
WEBHOOK_HE_REGEXP_DOMAIN_FILTER: a regular expression to specify domains to watch, eg "mycompany\..*"
WEBHOOK_HE_REGEXP_DOMAIN_FILTER_EXCLUDE: a regular expression to specify domains to ignore
```

Note that you must only use one of the two possible filtering mechanisms, either regexes or plain lists.

## Miscellaneous notes

- HE DNS does not allow the creation of wildcard records, so *don't use wildcards for your names*. In case a wildcard name slips through, the record creation will fail.

## Disclaimer

*Fact 1:* From [HE's TOS](https://dns.he.net/tos.html):

```
Hurricane reserves the right to remove or block any user provided records
that comes to its attention and that it believes, in its sole discretion, 
..., (ii) violates the rights of others, or degrades the performance of the service. 
(iii) abuse or fraudulently use the Service, (iv) use the Service in a
manner that causes interference to or tampers with another subscriber's or 
authorized user's use of the DNS service, ...
```

*Fact 2:* By default, external-dns has a polling period of _1 (one) minute_.

*You definitely do NOT want to be sending requests to HE every minute.* This is not AWS. The service must be used wisely. It's a good service that works well, so everybody should take care to not cause trouble.

To perform its actions, the webhook basically logs into the HE dashboard the same way a normal user would, so to avoid unnecessary traffic you should set a very high polling interval and enable the `triggerLoopOnEvent` option, so records will still be updated when there is a change on the k8s side. Especially if you don't expect to have external changes on the DNS side, you should really set a very high polling interval (hours, or days). In any case, definitely please set it (much) higher than the default 1 minute. *YOU ARE RESPONSIBLE FOR ALL TRAFFIC PRODUCED BY YOUR ACCOUNT (INCLUDING ABUSE AND EXCESSIVE USAGE) WHEN USING THIS WEBHOOK.* Configure it appropriately.
