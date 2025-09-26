<!--
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>
SPDX-License-Identifier: BSD-3-Clause
-->
## **CSI Snapshot Exporter**

[![Project Stage](https://docs.outscale.com/fr/userguide/_images/Project-Sandbox-yellow.svg)](https://docs.outscale.com/en/userguide/Open-Source-Projects.html) [![](https://dcbadge.limes.pink/api/server/HUVtY5gT6s?style=flat&theme=default-inverted)](https://discord.gg/HUVtY5gT6s)

<p align="center">
  <img alt="Kubernetes Logo" src="https://upload.wikimedia.org/wikipedia/commons/3/39/Kubernetes_logo_without_workmark.svg" width="120px">
</p>

---

## 🌐 Links

- Documentation: <https://docs.outscale.com/en/userguide/Exporting-a-Snapshot-to-a-Bucket.html>
- Project website: <https://github.com/outscale/csi-snapshot-exporter>
- Join our community on [Discord](https://discord.gg/HUVtY5gT6s)

---

## 📄 Table of Contents

- [Overview](#-overview)
- [Installation](#-installation)
- [Configuration](#-configuration)
- [Usage](#-usage)
- [Examples](#-examples)
- [License](#-license)
- [Contributing](#-contributing)

---

## 🧭 Overview

**CSI Snapshot Exporter** is a sidecar container to the CSI driver. It allows exporting Volume Snapshots to OOS.

---

## ⚙ Installation

Installation is usually done through the CSI Helm chart.

---

## 🛠 Configuration

See the CSI Helm chart configuration.

---

## 🚀 Usage

The following parameters may be added to a `VolumeSnapshotClass`:

* `exportToOOS` (boolean) - enable exports,
* `exportImageFormat` (qcow2 | raw) - the export format, defaults to qcow2,
* `exportBucket` (string) - required,
* `exportPrefix` (string) - optional.

The following annotations will be added to `VolumeSnapshotContent` resources:

* `bsu.csi.outscale.com/export-task` - the id of the export task (e.g., `snap-export-12d8b47d`),
* `bsu.csi.outscale.com/export-state` - the state of the export task (`pending`, `active`, `completed`, `cancelled` or `failed`),
* `bsu.csi.outscale.com/export-path` - the path (including `exportPrefix`) of the file exported in the OOS bucket.

Placeholders may be added to `exportPrefix`:

* `{date}` will be replaced by the date, using the `YYYY-MM-DD` format,
* `{vs}` will be replaced by the name of the source `VolumeSnapshot`,
* `{ns}` will be replaced by the namespace of the source `VolumeSnapshot`.

---

## 💡 Examples

```
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: snapshot-exporter-test
driver: bsu.csi.outscale.com
parameters:
  exportBucket: my-snapshot-exports
  exportImageFormat: qcow2
  exportPrefix: {ns}/{vs}/{date}/
  exportToOOS: "true"
```

---

## 📜 License

**CSI Snapshot Exporter** is released under the BSD 3-Clause license.

© 2025 Outscale SAS

This project complies with the [REUSE Specification](https://reuse.software/).

See [LICENSES/](./LICENSES) directory for full license information.

---

## 🤝 Contributing

We welcome contributions!

Please read our [Contributing Guidelines](CONTRIBUTING.md) and [Code of Conduct](CODE_OF_CONDUCT.md) before submitting a pull request.
