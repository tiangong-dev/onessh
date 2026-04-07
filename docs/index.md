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
      text: 简体中文
      link: /zh/
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

| Section | Contents |
| --- | --- |
| [Guide](/guide/getting-started) | Install, commands overview, configuration |
| [Reference](/reference/) | Architecture and security (in depth) |

**Languages:** this site is available in [English](/) and [简体中文](/zh/).

Longer command examples and edge cases still live in the [README](https://github.com/tiangong-dev/onessh/blob/main/README.md) on GitHub ([中文 README](https://github.com/tiangong-dev/onessh/blob/main/README.zh-CN.md)).

## Quick example

```bash
onessh init
onessh add web1
onessh ls
onessh web1
```
