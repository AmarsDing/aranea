export type NavItem = {
  to: string;
  icon: string;
  labelKey: string;
  /** 路由 exact，默认 true */
  exact?: boolean;
};

export type NavGroup = {
  labelKey: string;
  items: NavItem[];
};

export const sideNavGroups: NavGroup[] = [
  {
    labelKey: "menu.groupMain",
    items: [
      { to: "/overview", icon: "dashboard", labelKey: "menu.overview" },
      { to: "/chat", icon: "chat", labelKey: "menu.chat" }
    ]
  },
  {
    labelKey: "menu.groupEntities",
    items: [
      { to: "/agents", icon: "smart_toy", labelKey: "menu.agents" },
      { to: "/settings/agent-categories", icon: "account_tree", labelKey: "menu.agentCategories" },
      { to: "/team", icon: "groups", labelKey: "menu.team" }
    ]
  },
  {
    labelKey: "menu.groupRegistry",
    items: [
      { to: "/models", icon: "model_training", labelKey: "menu.models" },
      { to: "/channels", icon: "hub", labelKey: "menu.channels" },
      { to: "/mcp", icon: "extension", labelKey: "menu.mcp" },
      { to: "/skills", icon: "psychology", labelKey: "menu.skills" },
      { to: "/tools", icon: "handyman", labelKey: "menu.tools" },
      { to: "/cron", icon: "schedule", labelKey: "menu.cron" }
    ]
  },
  {
    labelKey: "menu.groupOps",
    items: [
      { to: "/monitor/logs", icon: "monitor_heart", labelKey: "menu.monitor", exact: false },
      { to: "/shop", icon: "storefront", labelKey: "menu.shop" },
      { to: "/settings", icon: "settings", labelKey: "menu.settings" }
    ]
  }
];
