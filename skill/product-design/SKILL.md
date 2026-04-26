---
name: product-design
description: >-
  Applies localized Stitch-style DESIGN.md specs for product UI. Use when the
  user asks for a specific brand-inspired look (e.g. Airbnb, Linear, Stripe),
  product UI that matches a named design system in the getdesign/awesome list,
  or when working from aranea/skill/product-design/reference. Offline
  authority is always reference/<brand>/DESIGN.md; do not require network
  fetches to implement components.
---

# Product design (localized)

## Scope

- **目标**：在实现界面/视觉时，以本目录下**已落盘的** [Google Stitch 风格](https://stitch.withgoogle.com/docs/design-md/overview/) 的 `DESIGN.md` 为权威依据（颜色、字阶、组件、布局与禁忌），与 [awesome-design-md README](https://github.com/VoltAgent/awesome-design-md) 中各主题在 [getdesign.md](https://getdesign.md) 上的介绍页一一对应。
- **不做什么**：不代替项目根目录若存在的 `PROJECT.md` / 业务产品文档；与项目根 `DESIGN.md` 冲突时，**以用户或项目规则指定的那份为准**。

## 权威路径（无网可用）

- 单主题设计稿：`aranea/skill/product-design/reference/<brand id>/DESIGN.md`
- **索引表**（全量 69 个主题、说明、本地路径、getdesign 预览链）：`aranea/skill/product-design/reference/INDEX.md`
- 机读列表与哈希：`aranea/skill/product-design/reference/templates-manifest.json`
- 来源与许可说明：`aranea/skill/product-design/SOURCE.md`、`reference/LICENSE-THIRD-PARTY.txt`

## 索引速查

打开 `reference/INDEX.md` 按 brand id 查找；`brand id` 与 getdesign 的 URL 路径一致，例如：

- `airbnb` → `https://getdesign.md/airbnb/design-md` → 本地 `reference/airbnb/DESIGN.md`
- `linear.app` → `https://getdesign.md/linear.app/design-md` → 本地 `reference/linear.app/DESIGN.md`

（完整表格见 `INDEX.md`，勿逐条背 URL。）

## 工作流（给智能体）

1. 从用户或任务中确定 **brand id**（或从 `INDEX.md` / `templates-manifest.json` 里匹配说明）。
2. **读取** 对应的 `reference/<brand>/DESIGN.md`，按其中章节（色板、字阶、组件、响应式等）实现或评审 UI。
3. 不依赖 `npx getdesign` 或在线拉取；本仓库中文件已足。
4. 更新索引：在升级模板后于 `aranea/skill/product-design` 下执行 `node scripts/gen-index.mjs` 以刷新 `reference/INDEX.md`。

## 在 Cursor 中启用（可选）

Cursor 默认识别项目内 `.cursor/skills/<name>/SKILL.md`。若需被自动发现，可将本目录 **复制或符号链接** 为：

- `aranea/.cursor/skills/product-design/` → 本 `product-design` 目录内容

或继续在规则 / 对话中显式 `@` 本 `SKILL.md` 路径。
