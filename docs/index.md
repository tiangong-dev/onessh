---
layout: home

hero:
  name: OneSSH
  text: Encrypted SSH config, one master password
  tagline: Connect, exec, and copy files without retyping credentials. Safe to store in a public Git repo.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Reference
      link: /reference/
    - theme: alt
      text: View on GitHub
      link: https://github.com/tiangong-dev/onessh

features:
  - title: Encrypted at rest
    details: AES-256-GCM with Argon2id. Only ENC[...] ciphertext is stored on disk.
  - title: Memory-only cache
    details: Local agent holds the master password for a TTL; optional capability hardening for IPC.
  - title: Full SSH workflow
    details: Interactive SSH, exec, scp-style cp, connectivity tests, batch by tag or glob.
---

## Documentation

| Section | What you will find |
| --- | --- |
| [Guide](/guide/getting-started) | Install, first host, commands overview, configuration |
| [Reference](/reference/) | Architecture and security design (detailed) |

The [README](https://github.com/tiangong-dev/onessh/blob/main/README.md) is the long-form command reference with examples. 简体中文见 [README.zh-CN](https://github.com/tiangong-dev/onessh/blob/main/README.zh-CN.md).

## Quick example

```bash
onessh init
onessh add web1
onessh ls
onessh web1
```
