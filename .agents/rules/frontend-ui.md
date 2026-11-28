---
alwaysApply: false
globs: "frontend/**/*.tsx"
---

# Frontend UI Rules

## shadcn 官方组件

1. 页面和业务组件中的基础 UI（按钮、输入框、选择器、对话框、抽屉、表格、标签页、提示、徽标、卡片、下拉菜单等）必须使用 shadcn 官方组件，从 `@/components/ui/*` 引入。
2. `@/components/ui` 中缺少需要的官方组件时，在 `frontend/` 目录下运行 `pnpm dlx shadcn@latest add <component>` 添加。不要手写同名组件，也不要从其他项目复制改写过的版本。项目配置见 `frontend/components.json`（style `new-york`，图标库 `lucide`）。
3. 不要用原生元素加 Tailwind 重新实现 shadcn 已有的组件（例如用 `<button>` 代替 `Button`，用 `<select>` 代替 `Select`，用自定义浮层代替 `Dialog`、`Popover` 或 `Tooltip`）。也不要引入 MUI、Ant Design、Headless UI 等其他组件库，或在业务代码中直接使用 `@radix-ui/*` 原语。
4. 需要定制样式或行为时，优先通过 `className`、`variant`、`size` 以及组件组合实现。确实需要扩展时，先在 `@/components/ui` 中按 shadcn 的写法新增 variant，不要改变已有 props 的语义，以免影响其他调用方。
5. 由多个 shadcn 组件组合出的业务组件放在 `@/components/` 或对应 feature 的 `components/` 下，不要放进 `@/components/ui`。`@/components/ui` 只放 shadcn 官方组件和对它们的少量通用扩展。

## 组件使用细节

1. When using `AutoComplete` or `AutoCompleteSelect` inside a `Dialog`, pass `portalContainer` pointing at the dialog container element to avoid scroll and layering issues.
