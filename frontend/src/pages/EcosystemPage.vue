<template>
  <q-page class="q-pa-md">
    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-8">
        <q-card flat bordered class="full-height">
          <q-card-section>
            <div class="text-h6">生态市场</div>
            <div class="text-caption text-grey-7">聚合 Agent、Skill、MCP Server、Channel 模板与后续商业化入口。</div>
          </q-card-section>
          <q-separator />
          <q-card-section class="row q-col-gutter-md">
            <div v-for="item in cards" :key="item.title" class="col-12 col-sm-6">
              <q-card flat bordered>
                <q-card-section class="row items-start q-gutter-md no-wrap">
                  <q-avatar color="primary" text-color="white" :icon="item.icon" />
                  <div>
                    <div class="text-subtitle1 text-weight-medium">{{ item.title }}</div>
                    <div class="text-caption text-grey-7">{{ item.caption }}</div>
                  </div>
                </q-card-section>
              </q-card>
            </div>
          </q-card-section>
        </q-card>
      </div>
      <div class="col-12 col-lg-4">
        <q-card flat bordered>
          <q-card-section>
            <div class="text-subtitle1 text-weight-medium">上线策略</div>
            <div class="text-body2 text-grey-7 q-mt-sm">
              当前先提供信息架构壳层。交易、评分、发布审核、安装流水和权限策略需要产品规则明确后再落库实现。
            </div>
          </q-card-section>
        </q-card>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
const cards = [
  { icon: "smart_toy", title: "Agent 模板", caption: "沉淀可复用的行业 Agent 与编排方案。" },
  { icon: "psychology", title: "Skill 包", caption: "发现、导入并版本化管理 Skill。" },
  { icon: "extension", title: "MCP Server", caption: "集中管理工具服务器与连接测试。" },
  { icon: "hub", title: "Channel 模板", caption: "复用消息渠道、认证方式和部署配置。" }
];
</script>
