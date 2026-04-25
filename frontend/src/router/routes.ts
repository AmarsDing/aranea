import type { RouteRecordRaw } from "vue-router";
import MainLayout from "../layouts/MainLayout.vue";
import ChatPage from "../pages/ChatPage.vue";
import AgentsPage from "../pages/AgentsPage.vue";
import AgentSettingsPage from "../pages/AgentSettingsPage.vue";
import MonitorPage from "../pages/MonitorPage.vue";
import GenericPage from "../pages/GenericPage.vue";
import OverviewPage from "../pages/OverviewPage.vue";
import ResourceManagerPage from "../pages/ResourceManagerPage.vue";
import EcosystemPage from "../pages/EcosystemPage.vue";
import AgentCategoriesPage from "../pages/AgentCategoriesPage.vue";
import SkillsPage from "../pages/SkillsPage.vue";
import SkillRunsPage from "../pages/SkillRunsPage.vue";

export const routes: RouteRecordRaw[] = [
  {
    path: "/",
    component: MainLayout,
    children: [
      { path: "", redirect: "/overview" },
      { path: "overview", name: "overview", component: OverviewPage },
      { path: "chat", name: "chat", component: ChatPage },
      { path: "agents", name: "agents", component: AgentsPage },
      { path: "settings/agent-categories", name: "agent-categories", component: AgentCategoriesPage },
      { path: "agents/:id/settings", name: "agent-settings", component: AgentSettingsPage },
      { path: "team", name: "team", component: GenericPage, meta: { titleKey: "menu.team" } },
      {
        path: "models",
        name: "models",
        component: ResourceManagerPage,
        meta: { resource: "llm-provider-models", title: "模型管理", subtitle: "维护 Provider/Model 可用清单与模型校验来源。" }
      },
      {
        path: "channels",
        name: "channels",
        component: ResourceManagerPage,
        meta: { resource: "channels", title: "Channel 管理", subtitle: "管理外部消息渠道、凭据引用与 Agent 绑定配置。" }
      },
      {
        path: "mcp",
        name: "mcp",
        component: ResourceManagerPage,
        meta: { resource: "mcp-servers", title: "MCP 管理", subtitle: "登记 MCP Server、连接配置与启用状态。" }
      },
      {
        path: "skills",
        name: "skills",
        component: SkillsPage
      },
      {
        path: "skills/runs",
        name: "skill-runs",
        component: SkillRunsPage
      },
      {
        path: "tools",
        name: "tools",
        component: ResourceManagerPage,
        meta: { resource: "hooks", title: "Hook 管理", subtitle: "管理全局 Hook、Agent 绑定和执行开关。" }
      },
      {
        path: "cron",
        name: "cron",
        component: ResourceManagerPage,
        meta: { resource: "cron-tasks", title: "Cron 管理", subtitle: "管理定时任务、Agent 关联与执行配置。" }
      },
      { path: "monitor/logs", name: "monitor-logs", component: MonitorPage },
      { path: "shop", name: "shop", component: EcosystemPage },
      { path: "settings", name: "settings", component: GenericPage, meta: { titleKey: "menu.settings" } }
    ]
  }
];
