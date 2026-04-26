import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ref = path.join(__dirname, "..", "reference");
const manifest = JSON.parse(
  fs.readFileSync(path.join(ref, "templates-manifest.json"), "utf8"),
);

const lines = [
  "# product-design 索引（getdesign 主题）",
  "",
  "与 [awesome-design-md README](https://github.com/VoltAgent/awesome-design-md) 中各主题链接的 getdesign 展示页一一对应；**离线实现 UI 时以本目录下各子文件夹中的 `DESIGN.md` 为准**（`INDEX.md` 与 `airbnb/DESIGN.md` 等同级）。",
  "",
  "| brand id | 说明 | 本地设计稿 | getdesign 预览 |",
  "|---|---|---|---|",
];

for (const x of manifest) {
  const u = `https://getdesign.md/${x.brand}/design-md`;
  const p = "`" + x.brand + "/DESIGN.md`";
  const desc = String(x.description).replace(/\|/g, "/");
  lines.push(`| ${x.brand} | ${desc} | ${p} | [链接](${u}) |`);
}

fs.writeFileSync(path.join(ref, "INDEX.md"), lines.join("\n") + "\n", "utf8");
console.log("Wrote", path.join(ref, "INDEX.md"));
