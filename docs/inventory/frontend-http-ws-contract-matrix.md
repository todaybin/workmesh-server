# 前端 HTTP/WS 合约测试矩阵

> 生成时间：2026-09-05T16:11:25.763Z  
> 生成器：`test/contract/frontend-contract-matrix.mjs`  
> 本文和 JSON 只生成验收计划，不发送业务请求；所有条目保持 `not-run`。

## 统计

| 项目 | 数量/状态 |
| --- | ---: |
| 主菜单 | 13 |
| 接口案例 | 389 |
| HTTP | 385 |
| WS | 4 |
| 同路径/操作族 | 41 |
| 当前状态 | `not-run: 389` |

## 13 个主菜单接口分组

同一个共享 API 可能被多个菜单页面调用，因此各菜单的接口数是“引用数”，不应直接相加作为 389 条唯一接口总数；唯一接口总数以 JSON 的 `summary.endpointCount` 为准。

| 主菜单 | 路径 | 路由数 | 接口数 | HTTP | WS | 状态 |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| menu.home | `/` | 1 | 137 | 136 | 1 | not-run |
| menu.advanced | `/advanced` | 10 | 23 | 23 | 0 | not-run |
| menu.aiTools | `/ai` | 7 | 137 | 137 | 0 | not-run |
| menu.apps | `/apps` | 6 | 45 | 45 | 0 | not-run |
| menu.container | `/containers` | 12 | 87 | 87 | 0 | not-run |
| menu.cronjob | `/cronjobs` | 5 | 194 | 194 | 0 | not-run |
| menu.database | `/databases` | 12 | 43 | 42 | 1 | not-run |
| menu.system | `/hosts` | 14 | 111 | 108 | 3 | not-run |
| menu.logs | `/logs` | 9 | 93 | 93 | 0 | not-run |
| menu.settings | `/settings` | 10 | 53 | 53 | 0 | not-run |
| menu.terminal | `/terminal` | 2 | 23 | 23 | 0 | not-run |
| menu.toolbox | `/toolbox` | 9 | 41 | 41 | 0 | not-run |
| menu.website | `/websites` | 11 | 130 | 130 | 0 | not-run |

## 同一路由多操作/参数变体族

这些分组用于后续真实测试时逐项检查同一路径的 method、operation 以及 `type/operate/logType/source/scope` 变体；静态发现不代表服务端已通过。

| 菜单 | 基础路径 | 变体数 | 变体 | 状态 |
| --- | --- | ---: | --- | --- |
| menu.aiTools | `/ai/agents/agent` | 2 | `/api/v2/ai/agents/agent/create`<br>`/api/v2/ai/agents/agent/list` | not-run |
| menu.aiTools | `/ai/agents/plugins` | 2 | `/api/v2/ai/agents/plugins/list`<br>`/api/v2/ai/agents/plugins/search` | not-run |
| menu.aiTools | `/ai/agents/skills` | 2 | `/api/v2/ai/agents/skills/list`<br>`/api/v2/ai/agents/skills/search` | not-run |
| menu.aiTools | `/ai/gpu` | 2 | `/api/v2/ai/gpu/options`<br>`/api/v2/ai/gpu/search` | not-run |
| menu.home | `/alert` | 4 | `/api/v2/alert`<br>`/api/v2/alert/del`<br>`/api/v2/alert/status`<br>`/api/v2/alert/update` | not-run |
| menu.home | `/alert/config` | 3 | `/api/v2/alert/config/del`<br>`/api/v2/alert/config/info`<br>`/api/v2/alert/config/update` | not-run |
| menu.apps | `/apps` | 2 | `/api/v2/apps/install`<br>`/api/v2/apps/search` | not-run |
| menu.apps | `/apps/installed` | 2 | `/api/v2/apps/installed/check`<br>`/api/v2/apps/installed/sync` | not-run |
| menu.container | `/containers` | 2 | `/api/v2/containers/info`<br>`/api/v2/containers/status` | not-run |
| menu.home | `/core/auth/current` | 2 | `/api/v2/core/auth/current`<br>`/api/v2/core/auth/current/update` | not-run |
| menu.home | `/core/commands` | 2 | `/api/v2/core/commands`<br>`/api/v2/core/commands/tree` | not-run |
| menu.home | `/core/enterprise/licenses` | 2 | `/api/v2/core/enterprise/licenses/info`<br>`/api/v2/core/enterprise/licenses/status` | not-run |
| menu.home | `/core/settings/ssl` | 2 | `/api/v2/core/settings/ssl/download`<br>`/api/v2/core/settings/ssl/info` | not-run |
| menu.database | `/databases/redis` | 2 | `/api/v2/databases/redis/check`<br>`/api/v2/databases/redis/status` | not-run |
| menu.home | `/files` | 7 | `/api/v2/files`<br>`/api/v2/files/check`<br>`/api/v2/files/del`<br>`/api/v2/files/download`<br>`/api/v2/files/search`<br>`/api/v2/files/tree`<br>`/api/v2/files/upload` | not-run |
| menu.home | `/files/favorite` | 2 | `/api/v2/files/favorite`<br>`/api/v2/files/favorite/del` | not-run |
| menu.home | `/files/share` | 3 | `/api/v2/files/share/create`<br>`/api/v2/files/share/del`<br>`/api/v2/files/share/info` | not-run |
| menu.system | `/hosts` | 2 | `/api/v2/hosts`<br>`/api/v2/hosts/info` | not-run |
| menu.system | `/hosts/firewall/rules` | 5 | `/api/v2/hosts/firewall/rules`<br>`/api/v2/hosts/firewall/rules/check`<br>`/api/v2/hosts/firewall/rules/delete`<br>`/api/v2/hosts/firewall/rules/search`<br>`/api/v2/hosts/firewall/rules/sync` | not-run |
| menu.system | `/hosts/tool` | 2 | `/api/v2/hosts/tool/operate`<br>`/api/v2/hosts/tool/status` | not-run |
| menu.system | `/hosts/tool/supervisor/process/file` | 2 | `/api/v2/hosts/tool/supervisor/process/file`<br>`/api/v2/hosts/tool/supervisor/process/file/get` | not-run |
| menu.website | `/openresty` | 2 | `/api/v2/openresty`<br>`/api/v2/openresty/status` | not-run |
| menu.home | `/runtimes` | 4 | `/api/v2/runtimes`<br>`/api/v2/runtimes/del`<br>`/api/v2/runtimes/operate`<br>`/api/v2/runtimes/update` | not-run |
| menu.home | `/runtimes/node/modules` | 2 | `/api/v2/runtimes/node/modules`<br>`/api/v2/runtimes/node/modules/operate` | not-run |
| menu.home | `/runtimes/php` | 2 | `/api/v2/runtimes/php/config`<br>`/api/v2/runtimes/php/update` | not-run |
| menu.home | `/runtimes/php/extensions` | 3 | `/api/v2/runtimes/php/extensions`<br>`/api/v2/runtimes/php/extensions/del`<br>`/api/v2/runtimes/php/extensions/update` | not-run |
| menu.website | `/websites` | 6 | `/api/v2/websites`<br>`/api/v2/websites/check`<br>`/api/v2/websites/config`<br>`/api/v2/websites/del`<br>`/api/v2/websites/list`<br>`/api/v2/websites/options` | not-run |
| menu.website | `/websites/acme` | 3 | `/api/v2/websites/acme`<br>`/api/v2/websites/acme/del`<br>`/api/v2/websites/acme/update` | not-run |
| menu.website | `/websites/auths` | 2 | `/api/v2/websites/auths`<br>`/api/v2/websites/auths/update` | not-run |
| menu.website | `/websites/ca` | 3 | `/api/v2/websites/ca`<br>`/api/v2/websites/ca/del`<br>`/api/v2/websites/ca/download` | not-run |
| menu.website | `/websites/dir` | 2 | `/api/v2/websites/dir`<br>`/api/v2/websites/dir/update` | not-run |
| menu.website | `/websites/dns` | 3 | `/api/v2/websites/dns`<br>`/api/v2/websites/dns/del`<br>`/api/v2/websites/dns/update` | not-run |
| menu.website | `/websites/domains` | 3 | `/api/v2/websites/domains`<br>`/api/v2/websites/domains/del/`<br>`/api/v2/websites/domains/update` | not-run |
| menu.website | `/websites/leech` | 2 | `/api/v2/websites/leech`<br>`/api/v2/websites/leech/update` | not-run |
| menu.website | `/websites/log` | 2 | `/api/v2/websites/log/operate`<br>`/api/v2/websites/log/search` | not-run |
| menu.website | `/websites/proxies` | 4 | `/api/v2/websites/proxies`<br>`/api/v2/websites/proxies/delete`<br>`/api/v2/websites/proxies/status`<br>`/api/v2/websites/proxies/update` | not-run |
| menu.website | `/websites/redirect` | 2 | `/api/v2/websites/redirect`<br>`/api/v2/websites/redirect/update` | not-run |
| menu.website | `/websites/rewrite` | 2 | `/api/v2/websites/rewrite`<br>`/api/v2/websites/rewrite/update` | not-run |
| menu.website | `/websites/ssl` | 6 | `/api/v2/websites/ssl`<br>`/api/v2/websites/ssl/del`<br>`/api/v2/websites/ssl/download`<br>`/api/v2/websites/ssl/list`<br>`/api/v2/websites/ssl/update`<br>`/api/v2/websites/ssl/upload` | not-run |
| menu.website | `/websites/templates` | 5 | `/api/v2/websites/templates`<br>`/api/v2/websites/templates/del`<br>`/api/v2/websites/templates/get`<br>`/api/v2/websites/templates/update`<br>`/api/v2/websites/templates/upload` | not-run |
| menu.website | `/websites/templates/outputs` | 3 | `/api/v2/websites/templates/outputs`<br>`/api/v2/websites/templates/outputs/del`<br>`/api/v2/websites/templates/outputs/get` | not-run |

## 案例字段来源

| ID | 菜单 | 协议 | 路径 | 方法 | 前端函数 | 响应类型 | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| frontend-contract-001 | menu.aiTools | HTTP | `/api/v2/ai/accounts/models` | POST | getAgentAccountModels | AI.AgentAccountModel[] | not-run |
| frontend-contract-002 | menu.aiTools | HTTP | `/api/v2/ai/accounts/models/discover` | POST | discoverAgentAccountModels | AI.AgentAccountModel[] | not-run |
| frontend-contract-003 | menu.aiTools | HTTP | `/api/v2/ai/accounts/providers` | GET | getAgentProviders | AI.ProviderInfo[] | not-run |
| frontend-contract-004 | menu.aiTools | HTTP | `/api/v2/ai/agents` | POST | createAgent | AI.AgentItem | not-run |
| frontend-contract-005 | menu.aiTools | HTTP | `/api/v2/ai/agents/agent/channels` | POST | getAgentRoleChannels | AI.AgentRoleChannelItem[] | not-run |
| frontend-contract-006 | menu.aiTools | HTTP | `/api/v2/ai/agents/agent/create` | POST | createAgentRole | AI.AgentRoleCreateResp | not-run |
| frontend-contract-007 | menu.aiTools | HTTP | `/api/v2/ai/agents/agent/list` | POST | getConfiguredAgentRoles | AI.AgentConfiguredAgentItem[] | not-run |
| frontend-contract-008 | menu.aiTools | HTTP | `/api/v2/ai/agents/agent/md/list` | POST | getAgentRoleMarkdownFiles | AI.AgentRoleMarkdownFileItem[] | not-run |
| frontend-contract-009 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/dingtalk/get` | POST | getAgentDingTalkConfig | AI.AgentDingTalkConfig | not-run |
| frontend-contract-010 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/discord/get` | POST | getAgentDiscordConfig | AI.AgentDiscordConfig | not-run |
| frontend-contract-011 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/feishu/get` | POST | getAgentFeishuConfig | AI.AgentFeishuConfig | not-run |
| frontend-contract-012 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/qqbot/get` | POST | getAgentQQBotConfig | AI.AgentQQBotConfig | not-run |
| frontend-contract-013 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/telegram/get` | POST | getAgentTelegramConfig | AI.AgentTelegramConfig | not-run |
| frontend-contract-014 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/wecom/get` | POST | getAgentWecomConfig | AI.AgentWecomConfig | not-run |
| frontend-contract-015 | menu.aiTools | HTTP | `/api/v2/ai/agents/channel/weixin/get` | POST | getAgentWeixinConfig | AI.AgentWeixinConfig | not-run |
| frontend-contract-016 | menu.aiTools | HTTP | `/api/v2/ai/agents/config-file/get` | POST | getAgentConfigFile | AI.AgentConfigFile | not-run |
| frontend-contract-017 | menu.aiTools | HTTP | `/api/v2/ai/agents/delete/check` | POST | deleteAgentCheck | App.AppInstallResource[] | not-run |
| frontend-contract-018 | menu.aiTools | HTTP | `/api/v2/ai/agents/hermes/chat/sessions` | POST | getAgentHermesChatSessions | AI.AgentHermesChatSessionItem[] | not-run |
| frontend-contract-019 | menu.aiTools | HTTP | `/api/v2/ai/agents/model/get` | POST | getAgentModelConfig | AI.AgentModelConfig | not-run |
| frontend-contract-020 | menu.aiTools | HTTP | `/api/v2/ai/agents/other/get` | POST | getAgentOtherConfig | AI.AgentOtherConfig | not-run |
| frontend-contract-021 | menu.aiTools | HTTP | `/api/v2/ai/agents/overview` | POST | getAgentOverview | AI.AgentOverview | not-run |
| frontend-contract-022 | menu.aiTools | HTTP | `/api/v2/ai/agents/plugin/check` | POST | checkAgentPlugin | AI.AgentPluginStatus | not-run |
| frontend-contract-023 | menu.aiTools | HTTP | `/api/v2/ai/agents/plugins/list` | POST | listAgentPlugins | AI.AgentPluginItem[] | not-run |
| frontend-contract-024 | menu.aiTools | HTTP | `/api/v2/ai/agents/plugins/search` | POST | searchAgentPlugins | AI.AgentPluginSearchItem[] | not-run |
| frontend-contract-025 | menu.aiTools | HTTP | `/api/v2/ai/agents/security/get` | POST | getAgentSecurityConfig | AI.AgentSecurityConfig | not-run |
| frontend-contract-026 | menu.aiTools | HTTP | `/api/v2/ai/agents/skills/list` | POST | listAgentSkills | AI.AgentSkillItem[] | not-run |
| frontend-contract-027 | menu.aiTools | HTTP | `/api/v2/ai/agents/skills/search` | POST | searchAgentSkills | AI.AgentSkillSearchItem[] | not-run |
| frontend-contract-028 | menu.aiTools | HTTP | `/api/v2/ai/domain/get` | POST | getBindDomain | AI.BindDomainRes | not-run |
| frontend-contract-029 | menu.aiTools | HTTP | `/api/v2/ai/gpu/load` | GET | loadGPUInfo | AI.Info | not-run |
| frontend-contract-030 | menu.aiTools | HTTP | `/api/v2/ai/gpu/options` | GET | getGPUOptions | AI.MonitorGPUOptions | not-run |
| frontend-contract-031 | menu.aiTools | HTTP | `/api/v2/ai/gpu/search` | POST | loadGPUMonitor | AI.MonitorGPUData | not-run |
| frontend-contract-032 | menu.aiTools | HTTP | `/api/v2/ai/mcp/domain/get` | GET | getMcpDomain | AI.McpDomainRes | not-run |
| frontend-contract-033 | menu.aiTools | HTTP | `/api/v2/ai/mcp/server/connection/test` | POST | testMcpServerConnection | AI.McpServerConnectionTestRes | not-run |
| frontend-contract-034 | menu.aiTools | HTTP | `/api/v2/ai/mcp/server/detail` | POST | loadMcpServerDetail | AI.McpServer | not-run |
| frontend-contract-035 | menu.aiTools | HTTP | `/api/v2/ai/mcp/server/status/sync` | POST | syncMcpServerStatus | AI.McpServerStatus[] | not-run |
| frontend-contract-036 | menu.aiTools | HTTP | `/api/v2/ai/ollama/model/load` | POST | loadOllamaModel | string | not-run |
| frontend-contract-037 | menu.home | HTTP | `/api/v2/alert` | POST | CreateAlert | any | not-run |
| frontend-contract-038 | menu.home | HTTP | `/api/v2/alert/clams/list` | GET | ListClams | Alert.ClamsDTO[] | not-run |
| frontend-contract-039 | menu.home | HTTP | `/api/v2/alert/config/del` | POST | DeleteAlertConfig | any | not-run |
| frontend-contract-040 | menu.home | HTTP | `/api/v2/alert/config/info` | POST | ListAlertConfigs | Alert.AlertConfigInfo[] | not-run |
| frontend-contract-041 | menu.home | HTTP | `/api/v2/alert/config/test` | POST | TestAlertConfig | any | not-run |
| frontend-contract-042 | menu.home | HTTP | `/api/v2/alert/config/update` | POST | UpdateAlertConfig | any | not-run |
| frontend-contract-043 | menu.home | HTTP | `/api/v2/alert/cronjob/list` | POST | ListCronJob | Alert.CronJobDTO[] | not-run |
| frontend-contract-044 | menu.home | HTTP | `/api/v2/alert/del` | POST | DeleteAlert | any | not-run |
| frontend-contract-045 | menu.home | HTTP | `/api/v2/alert/disks/list` | GET | ListDisks | Alert.DisksDTO[] | not-run |
| frontend-contract-046 | menu.home | HTTP | `/api/v2/alert/logs/clean` | POST | CleanAlertLogs | any | not-run |
| frontend-contract-047 | menu.home | HTTP | `/api/v2/alert/status` | POST | UpdateAlertStatus | any | not-run |
| frontend-contract-048 | menu.home | HTTP | `/api/v2/alert/update` | POST | UpdateAlert | any | not-run |
| frontend-contract-049 | menu.apps | HTTP | `/api/v2/apps/` | GET | getAppByKey | App.AppDTO | not-run |
| frontend-contract-050 | menu.apps | HTTP | `/api/v2/apps/detail/:param/:param/:param:param` | GET | getAppDetail | App.AppDetail | not-run |
| frontend-contract-051 | menu.apps | HTTP | `/api/v2/apps/details/:param` | GET | getAppDetailByID | App.AppDetail | not-run |
| frontend-contract-052 | menu.apps | HTTP | `/api/v2/apps/ignored/detail` | GET | getIgnoredApp | App.IgnoredApp | not-run |
| frontend-contract-053 | menu.apps | HTTP | `/api/v2/apps/install` | POST | installApp | any | not-run |
| frontend-contract-054 | menu.apps | HTTP | `/api/v2/apps/installed/check` | POST | checkAppInstalled | App.CheckInstalled | not-run |
| frontend-contract-055 | menu.apps | HTTP | `/api/v2/apps/installed/conf` | POST | getAppDefaultConfig | string | not-run |
| frontend-contract-056 | menu.apps | HTTP | `/api/v2/apps/installed/conninfo` | POST | getAppConnInfo | App.DatabaseConnInfo | not-run |
| frontend-contract-057 | menu.apps | HTTP | `/api/v2/apps/installed/delete/check/:param:param` | GET | appInstalledDeleteCheck | App.AppInstallResource[] | not-run |
| frontend-contract-058 | menu.apps | HTTP | `/api/v2/apps/installed/ignore` | POST | ignoreUpgrade | any | not-run |
| frontend-contract-059 | menu.apps | HTTP | `/api/v2/apps/installed/info/:param:param` | GET | getAppInstalledByID | App.AppInstalledInfo | not-run |
| frontend-contract-060 | menu.apps | HTTP | `/api/v2/apps/installed/loadport` | POST | getAppPort | number | not-run |
| frontend-contract-061 | menu.apps | HTTP | `/api/v2/apps/installed/op:param` | POST | installedOp | any | not-run |
| frontend-contract-062 | menu.apps | HTTP | `/api/v2/apps/installed/params/:param` | GET | getAppInstallParams | App.AppConfig | not-run |
| frontend-contract-063 | menu.apps | HTTP | `/api/v2/apps/installed/params/update` | POST | updateAppInstallParams | any | not-run |
| frontend-contract-064 | menu.apps | HTTP | `/api/v2/apps/installed/port/change` | POST | changePort | any | not-run |
| frontend-contract-065 | menu.apps | HTTP | `/api/v2/apps/installed/sync` | POST | syncInstalledApp | any | not-run |
| frontend-contract-066 | menu.apps | HTTP | `/api/v2/apps/installed/update/versions:param` | POST | getAppUpdateVersions | any | not-run |
| frontend-contract-067 | menu.apps | HTTP | `/api/v2/apps/search` | POST | searchApp | App.AppResPage | not-run |
| frontend-contract-068 | menu.apps | HTTP | `/api/v2/apps/services/:param:param` | GET | getAppService | App.AppService[] | not-run |
| frontend-contract-069 | menu.apps | HTTP | `/api/v2/apps/tags` | GET | getAppTags | App.Tag[] | not-run |
| frontend-contract-070 | menu.home | HTTP | `/api/v2/backups/conn/check` | POSTLOCALNODE, POST | checkBackup | Backup.CheckResult | not-run |
| frontend-contract-071 | menu.home | HTTP | `/api/v2/backups/local:param` | GET | getLocalBackupDir | string | not-run |
| frontend-contract-072 | menu.home | HTTP | `/api/v2/backups/record/download:param` | POST | downloadBackupRecord | string | not-run |
| frontend-contract-073 | menu.container | HTTP | `/api/v2/containers/compose` | POST | upCompose | string | not-run |
| frontend-contract-074 | menu.container | HTTP | `/api/v2/containers/compose/env` | POST | loadComposeEnv | string | not-run |
| frontend-contract-075 | menu.container | HTTP | `/api/v2/containers/compose/test` | POST | testCompose | boolean | not-run |
| frontend-contract-076 | menu.container | HTTP | `/api/v2/containers/daemonjson` | GET | loadDaemonJson | Container.DaemonJsonConf | not-run |
| frontend-contract-077 | menu.container | HTTP | `/api/v2/containers/daemonjson/file` | GET | loadDaemonJsonFile | string | not-run |
| frontend-contract-078 | menu.container | HTTP | `/api/v2/containers/docker/status` | GET | loadDockerStatus | Container.DockerStatus | not-run |
| frontend-contract-079 | menu.container | HTTP | `/api/v2/containers/download/log` | DOWNLOAD | DownloadFile | BlobPart | not-run |
| frontend-contract-080 | menu.container | HTTP | `/api/v2/containers/files/content` | POST | getContainerFileContent | Container.ContainerFileContent | not-run |
| frontend-contract-081 | menu.container | HTTP | `/api/v2/containers/files/download` | DOWNLOAD | downloadContainerFile | BlobPart | not-run |
| frontend-contract-082 | menu.container | HTTP | `/api/v2/containers/files/size` | POST | getContainerFileSize | number | not-run |
| frontend-contract-083 | menu.container | HTTP | `/api/v2/containers/image/build` | POST | imageBuild | string | not-run |
| frontend-contract-084 | menu.container | HTTP | `/api/v2/containers/image/pull` | POST | imagePull | string | not-run |
| frontend-contract-085 | menu.container | HTTP | `/api/v2/containers/image/push` | POST | imagePush | string | not-run |
| frontend-contract-086 | menu.container | HTTP | `/api/v2/containers/info` | POST | loadContainerInfo | Container.ContainerHelper | not-run |
| frontend-contract-087 | menu.container | HTTP | `/api/v2/containers/inspect` | POST | inspect | string | not-run |
| frontend-contract-088 | menu.container | HTTP | `/api/v2/containers/item/stats` | POST | containerItemStats | Container.ContainerItemStats | not-run |
| frontend-contract-089 | menu.container | HTTP | `/api/v2/containers/limit` | GET | loadResourceLimit | Container.ResourceLimit | not-run |
| frontend-contract-090 | menu.container | HTTP | `/api/v2/containers/repo` | GET | listImageRepo | Container.RepoOptions | not-run |
| frontend-contract-091 | menu.container | HTTP | `/api/v2/containers/search/log?compose=:param&since=:param&tail=:param&follow=:param&timestamp=:param&operateNode=:param` | GET | — | — | not-run |
| frontend-contract-092 | menu.container | HTTP | `/api/v2/containers/search/log?container=:param&since=:param&tail=:param&follow=:param&timestamp=:param&operateNode=:param` | GET | — | — | not-run |
| frontend-contract-093 | menu.container | HTTP | `/api/v2/containers/stats/:param` | GET | containerStats | Container.ContainerStats | not-run |
| frontend-contract-094 | menu.container | HTTP | `/api/v2/containers/status` | GET | loadContainerStatus | Container.ContainerStatus | not-run |
| frontend-contract-095 | menu.container | HTTP | `/api/v2/containers/template` | GET | listComposeTemplate | Container.TemplateInfo | not-run |
| frontend-contract-096 | menu.home | HTTP | `/api/v2/core/auth/api/generate` | POST | generateApiKey | string | not-run |
| frontend-contract-097 | menu.home | HTTP | `/api/v2/core/auth/captcha` | GET | getCaptcha | Login.ResCaptcha | not-run |
| frontend-contract-098 | menu.home | HTTP | `/api/v2/core/auth/current` | GET | getUserInfo | Login.AuthInfo | not-run |
| frontend-contract-099 | menu.home | HTTP | `/api/v2/core/auth/current/update` | POST | updateUserInfo | any | not-run |
| frontend-contract-100 | menu.home | HTTP | `/api/v2/core/auth/ldap/status` | GET | ldapStatusApi | Login.LDAPStatus | not-run |
| frontend-contract-101 | menu.home | HTTP | `/api/v2/core/auth/login` | POST | loginApi | Login.ResLogin | not-run |
| frontend-contract-102 | menu.home | HTTP | `/api/v2/core/auth/logout` | POST | logOutApi | Login.LogOutResponse | not-run |
| frontend-contract-103 | menu.home | HTTP | `/api/v2/core/auth/mfa` | POST | loadMFA | Login.MFAInfo | not-run |
| frontend-contract-104 | menu.home | HTTP | `/api/v2/core/auth/mfalogin` | POST | mfaLoginApi | Login.ResLogin | not-run |
| frontend-contract-105 | menu.home | HTTP | `/api/v2/core/auth/oidc/begin` | POST | oidcBeginApi | Login.OIDCBeginResponse | not-run |
| frontend-contract-106 | menu.home | HTTP | `/api/v2/core/auth/oidc/finish` | POST | oidcFinishApi | Login.ResLogin | not-run |
| frontend-contract-107 | menu.home | HTTP | `/api/v2/core/auth/oidc/status` | GET | oidcStatusApi | Login.OIDCStatus | not-run |
| frontend-contract-108 | menu.home | HTTP | `/api/v2/core/auth/passkey/begin` | POST | passkeyBeginApi | Login.PasskeyBeginResponse | not-run |
| frontend-contract-109 | menu.home | HTTP | `/api/v2/core/auth/passkey/finish` | POST | passkeyFinishApi | Login.ResLogin | not-run |
| frontend-contract-110 | menu.home | HTTP | `/api/v2/core/auth/passkey/register/begin` | POST | passkeyRegisterBegin | Login.PasskeyBeginResponse | not-run |
| frontend-contract-111 | menu.home | HTTP | `/api/v2/core/auth/saml2/begin` | POST | saml2BeginApi | Login.SAML2BeginResponse | not-run |
| frontend-contract-112 | menu.home | HTTP | `/api/v2/core/auth/saml2/finish` | POST | saml2FinishApi | Login.ResLogin | not-run |
| frontend-contract-113 | menu.home | HTTP | `/api/v2/core/auth/saml2/status` | GET | saml2StatusApi | Login.SAML2Status | not-run |
| frontend-contract-114 | menu.home | HTTP | `/api/v2/core/auth/setting` | GET | getLoginSetting | Login.LoginSetting | not-run |
| frontend-contract-115 | menu.home | HTTP | `/api/v2/core/auth/welcome` | GET | getWelcomePage | string | not-run |
| frontend-contract-116 | menu.home | HTTP | `/api/v2/core/backups/client/:param` | GET | getClientInfo | Backup.ClientInfo | not-run |
| frontend-contract-117 | menu.home | HTTP | `/api/v2/core/commands` | POST | addCommand | Command.CommandOperate | not-run |
| frontend-contract-118 | menu.home | HTTP | `/api/v2/core/commands/export` | POST | exportCommands | string | not-run |
| frontend-contract-119 | menu.home | HTTP | `/api/v2/core/commands/tree` | POST | getCommandTree | any | not-run |
| frontend-contract-120 | menu.home | HTTP | `/api/v2/core/enterprise/licenses/community-restore/status` | GET | — | — | not-run |
| frontend-contract-121 | menu.home | HTTP | `/api/v2/core/enterprise/licenses/info` | GET | — | — | not-run |
| frontend-contract-122 | menu.home | HTTP | `/api/v2/core/enterprise/licenses/status` | GET | — | — | not-run |
| frontend-contract-123 | menu.home | HTTP | `/api/v2/core/groups` | POST | createGroup | Group.GroupCreate | not-run |
| frontend-contract-124 | menu.home | HTTP | `/api/v2/core/licenses/master/status` | GET | — | — | not-run |
| frontend-contract-125 | menu.home | HTTP | `/api/v2/core/licenses/sms/info` | GET | — | — | not-run |
| frontend-contract-126 | menu.home | HTTP | `/api/v2/core/licenses/status` | GET | — | — | not-run |
| frontend-contract-127 | menu.home | HTTP | `/api/v2/core/script/run` | GET | — | — | not-run |
| frontend-contract-128 | menu.home | HTTP | `/api/v2/core/settings/apps/store/config:param` | GET | getAppStoreConfig | App.AppStoreConfig | not-run |
| frontend-contract-129 | menu.home | HTTP | `/api/v2/core/settings/memo` | GET | getMemo | string | not-run |
| frontend-contract-130 | menu.home | HTTP | `/api/v2/core/settings/search` | POST | getSettingInfo | Setting.SettingInfo | not-run |
| frontend-contract-131 | menu.home | HTTP | `/api/v2/core/settings/search/base` | POST | getSettingBaseInfo | Setting.SettingBaseInfo | not-run |
| frontend-contract-132 | menu.home | HTTP | `/api/v2/core/settings/ssl/download` | DOWNLOAD | downloadSSL | any | not-run |
| frontend-contract-133 | menu.home | HTTP | `/api/v2/core/settings/ssl/info` | GET | loadSSLInfo | Setting.SSLInfo | not-run |
| frontend-contract-134 | menu.home | HTTP | `/api/v2/core/settings/terminal/search` | POST | getTerminalInfo | Setting.TerminalInfo | not-run |
| frontend-contract-135 | menu.home | HTTP | `/api/v2/core/settings/upgrade` | GET | loadUpgradeInfo | Setting.UpgradeInfo | not-run |
| frontend-contract-136 | menu.home | HTTP | `/api/v2/core/settings/upgrade/notes` | POST | loadReleaseNotes | string | not-run |
| frontend-contract-137 | menu.home | HTTP | `/api/v2/core/xpack/alert/offline/sync` | POST | SyncOfflineAlert | any | not-run |
| frontend-contract-138 | menu.cronjob | HTTP | `/api/v2/cronjobs` | POST | addCronjob | Cronjob.CronjobOperate | not-run |
| frontend-contract-139 | menu.cronjob | HTTP | `/api/v2/cronjobs/export` | DOWNLOAD | exportCronjob | BlobPart | not-run |
| frontend-contract-140 | menu.cronjob | HTTP | `/api/v2/cronjobs/load/info` | POST | loadCronjobInfo | Cronjob.CronjobOperate | not-run |
| frontend-contract-141 | menu.cronjob | HTTP | `/api/v2/cronjobs/records/log` | POST | getRecordLog | string | not-run |
| frontend-contract-142 | menu.home | HTTP | `/api/v2/custom/app/config` | GET | getCurrentNodeCustomAppConfig | App.CustomAppStoreConfig | not-run |
| frontend-contract-143 | menu.home | HTTP | `/api/v2/dashboard/base/:param/:param` | GET | loadBaseInfo | Dashboard.BaseInfo | not-run |
| frontend-contract-144 | menu.home | HTTP | `/api/v2/dashboard/base/os` | GET | loadOsInfo | Dashboard.OsInfo | not-run |
| frontend-contract-145 | menu.home | HTTP | `/api/v2/dashboard/current/:param/:param` | GET | loadCurrentInfo | Dashboard.CurrentInfo | not-run |
| frontend-contract-146 | menu.database | HTTP | `/api/v2/databases/common/info` | POST | loadDBBaseInfo | Database.BaseInfo | not-run |
| frontend-contract-147 | menu.database | HTTP | `/api/v2/databases/common/load/file` | POST | loadDBFile | string | not-run |
| frontend-contract-148 | menu.database | HTTP | `/api/v2/databases/db/:param` | GET | getDatabase | Database.DatabaseInfo | not-run |
| frontend-contract-149 | menu.database | HTTP | `/api/v2/databases/db/check` | POST | checkDatabase | boolean | not-run |
| frontend-contract-150 | menu.database | HTTP | `/api/v2/databases/db/del/check` | POST | deleteCheckDatabase | Database.DBResource[] | not-run |
| frontend-contract-151 | menu.database | HTTP | `/api/v2/databases/grants/search` | POST | searchMysqlGrants | Database.MysqlGrant[] | not-run |
| frontend-contract-152 | menu.database | HTTP | `/api/v2/databases/mongodb/del/check` | POST | deleteCheckMongodbDB | Database.DBResource[] | not-run |
| frontend-contract-153 | menu.database | HTTP | `/api/v2/databases/mongodb/privileges` | POST | loadMongodbPrivileges | string | not-run |
| frontend-contract-154 | menu.database | HTTP | `/api/v2/databases/pg/del/check` | POST | deleteCheckPostgresqlDB | Database.DBResource[] | not-run |
| frontend-contract-155 | menu.database | HTTP | `/api/v2/databases/redis/check` | GET | checkRedisCli | boolean | not-run |
| frontend-contract-156 | menu.database | HTTP | `/api/v2/databases/redis/conf` | POST | loadRedisConf | Database.RedisConf | not-run |
| frontend-contract-157 | menu.database | HTTP | `/api/v2/databases/redis/persistence/conf` | POST | redisPersistenceConf | Database.RedisPersistenceConf | not-run |
| frontend-contract-158 | menu.database | HTTP | `/api/v2/databases/redis/status` | POST | loadRedisStatus | Database.RedisStatus | not-run |
| frontend-contract-159 | menu.database | HTTP | `/api/v2/databases/remote` | POST | loadRemoteAccess | boolean | not-run |
| frontend-contract-160 | menu.database | HTTP | `/api/v2/databases/status` | POST | loadMysqlStatus | Database.MysqlStatus | not-run |
| frontend-contract-161 | menu.database | HTTP | `/api/v2/databases/users/search` | POST | searchMysqlUsers | Database.MysqlUser[] | not-run |
| frontend-contract-162 | menu.database | HTTP | `/api/v2/databases/variables` | POST | loadMysqlVariables | Database.MysqlVariables | not-run |
| frontend-contract-163 | menu.home | HTTP | `/api/v2/files` | POST | createFile | File.File | not-run |
| frontend-contract-164 | menu.home | HTTP | `/api/v2/files/ai-search` | POST | fileAiSearch | File.FileAISearchResult | not-run |
| frontend-contract-165 | menu.home | HTTP | `/api/v2/files/batch/check` | POST | batchCheckFiles | File.ExistFileInfo[] | not-run |
| frontend-contract-166 | menu.home | HTTP | `/api/v2/files/batch/role` | POST | batchChangeRole | any | not-run |
| frontend-contract-167 | menu.home | HTTP | `/api/v2/files/check` | POST | checkFile | boolean | not-run |
| frontend-contract-168 | menu.home | HTTP | `/api/v2/files/chunkupload` | UPLOAD | chunkUploadFileData | File.File | not-run |
| frontend-contract-169 | menu.home | HTTP | `/api/v2/files/content` | POST | getFileContent | File.File | not-run |
| frontend-contract-170 | menu.home | HTTP | `/api/v2/files/convert` | POST | convertFiles | File.ConvertFile | not-run |
| frontend-contract-171 | menu.home | HTTP | `/api/v2/files/del` | POST | deleteFile | File.File | not-run |
| frontend-contract-172 | menu.home | HTTP | `/api/v2/files/del?operateNode=` | POST | deleteFileByNode | File.File | not-run |
| frontend-contract-173 | menu.home | HTTP | `/api/v2/files/depth/size` | POST | computeDepthDirSize | File.DepthDirSizeRes[] | not-run |
| frontend-contract-174 | menu.home | HTTP | `/api/v2/files/download` | DOWNLOAD | downloadFile | BlobPart | not-run |
| frontend-contract-175 | menu.home | HTTP | `/api/v2/files/favorite` | POST | addFavorite | any | not-run |
| frontend-contract-176 | menu.home | HTTP | `/api/v2/files/favorite/del` | POST | removeFavorite | any | not-run |
| frontend-contract-177 | menu.home | HTTP | `/api/v2/files/history/content` | POST | getFileHistoryContent | File.FileHistoryInfo | not-run |
| frontend-contract-178 | menu.home | HTTP | `/api/v2/files/history/restore` | POST | restoreFileHistory | File.File | not-run |
| frontend-contract-179 | menu.home | HTTP | `/api/v2/files/mode` | POST | changeFileMode | File.File | not-run |
| frontend-contract-180 | menu.home | HTTP | `/api/v2/files/mount` | POST | searchHostMount | Dashboard.DiskInfo[] | not-run |
| frontend-contract-181 | menu.home | HTTP | `/api/v2/files/move` | POST | moveFile | File.File | not-run |
| frontend-contract-182 | menu.home | HTTP | `/api/v2/files/owner` | POST | changeOwner | File.File | not-run |
| frontend-contract-183 | menu.home | HTTP | `/api/v2/files/preview` | POST | getPreviewContent | File.File | not-run |
| frontend-contract-184 | menu.home | HTTP | `/api/v2/files/read/:param:param` | POST | readByLine | any | not-run |
| frontend-contract-185 | menu.home | HTTP | `/api/v2/files/recycle/clear` | POST | clearRecycle | any | not-run |
| frontend-contract-186 | menu.home | HTTP | `/api/v2/files/recycle/reduce` | POST | reduceFile | any | not-run |
| frontend-contract-187 | menu.home | HTTP | `/api/v2/files/recycle/status` | GET | getRecycleStatus | string | not-run |
| frontend-contract-188 | menu.home | HTTP | `/api/v2/files/recycle/status?operateNode=` | GET | getRecycleStatusByNode | string | not-run |
| frontend-contract-189 | menu.home | HTTP | `/api/v2/files/remarks` | POST | batchGetFileRemarks | File.FileRemarksRes | not-run |
| frontend-contract-190 | menu.home | HTTP | `/api/v2/files/rename` | POST | renameRile | File.File | not-run |
| frontend-contract-191 | menu.home | HTTP | `/api/v2/files/save` | POST | saveFileContent | File.File | not-run |
| frontend-contract-192 | menu.home | HTTP | `/api/v2/files/search` | POST | getFilesList | File.File | not-run |
| frontend-contract-193 | menu.home | HTTP | `/api/v2/files/search?operateNode=` | POST | getFilesListByNode | File.File | not-run |
| frontend-contract-194 | menu.home | HTTP | `/api/v2/files/share/create` | POST | createFileShare | File.FileShareInfo | not-run |
| frontend-contract-195 | menu.home | HTTP | `/api/v2/files/share/del` | POST | removeFileShare | any | not-run |
| frontend-contract-196 | menu.home | HTTP | `/api/v2/files/share/detail` | POST | getFileShareDetail | File.FileShareInfo | null | not-run |
| frontend-contract-197 | menu.home | HTTP | `/api/v2/files/share/info` | GET | getPublicFileShareInfo | File.FileSharePublicInfo | not-run |
| frontend-contract-198 | menu.home | HTTP | `/api/v2/files/size` | POST | computeDirSize | File.DirSizeRes | not-run |
| frontend-contract-199 | menu.home | HTTP | `/api/v2/files/tree` | POST | getFilesTree | File.FileTree[] | not-run |
| frontend-contract-200 | menu.home | HTTP | `/api/v2/files/upload` | UPLOAD | uploadFileData | File.File | not-run |
| frontend-contract-201 | menu.home | HTTP | `/api/v2/files/user/group` | POST | searchUserGroup | File.UserGroupResponse | not-run |
| frontend-contract-202 | menu.home | HTTP | `/api/v2/files/wget` | POST | wgetFile | File.FileWgetRes | not-run |
| frontend-contract-203 | menu.home | HTTP | `/api/v2/files/wget/process/keys` | GET | fileWgetKeys | File.FileKeys | not-run |
| frontend-contract-204 | menu.home | HTTP | `/api/v2/groups` | POST | createAgentGroup | Group.GroupCreate | not-run |
| frontend-contract-205 | menu.system | HTTP | `/api/v2/hosts` | POSTLOCALNODE | — | — | not-run |
| frontend-contract-206 | menu.system | HTTP | `/api/v2/hosts/components/:param:param` | GET | getComponentInfo | Host.ComponentInfo | not-run |
| frontend-contract-207 | menu.system | HTTP | `/api/v2/hosts/diagnostics/goroutines` | GET | loadRuntimeGoroutines | Host.RuntimeGoroutineSnapshot | not-run |
| frontend-contract-208 | menu.system | HTTP | `/api/v2/hosts/diagnostics/profiles` | DOWNLOAD | loadRuntimeGoroutines | Blob | not-run |
| frontend-contract-209 | menu.system | HTTP | `/api/v2/hosts/diagnostics/summary` | GET | loadRuntimeDiagnosticsSummary | Host.RuntimeDiagnosticsSummary | not-run |
| frontend-contract-210 | menu.system | HTTP | `/api/v2/hosts/disks` | GET | listDisks | Host.CompleteDiskInfo | not-run |
| frontend-contract-211 | menu.system | HTTP | `/api/v2/hosts/firewall/base` | POST | loadFireBaseInfo | Firewall.FirewallBase | not-run |
| frontend-contract-212 | menu.system | HTTP | `/api/v2/hosts/firewall/docker/endpoints` | GET | loadDockerPublishedPorts | Firewall.DockerGuardContainer[] | not-run |
| frontend-contract-213 | menu.system | HTTP | `/api/v2/hosts/firewall/docker/operate` | POSTWITHCONFIG | operateDockerPortGuard | — | not-run |
| frontend-contract-214 | menu.system | HTTP | `/api/v2/hosts/firewall/docker/ports` | GET | loadDockerPortGuard | Firewall.DockerGuardList | not-run |
| frontend-contract-215 | menu.system | HTTP | `/api/v2/hosts/firewall/filter/operate` | POST | operateFilterChain | — | not-run |
| frontend-contract-216 | menu.system | HTTP | `/api/v2/hosts/firewall/forward/base` | POST | loadForwardBaseInfo | Firewall.FirewallBase | not-run |
| frontend-contract-217 | menu.system | HTTP | `/api/v2/hosts/firewall/forward/enable` | POST | enableForwarding | — | not-run |
| frontend-contract-218 | menu.system | HTTP | `/api/v2/hosts/firewall/rules` | POST | createFirewallRules | Firewall.CreateResponse | not-run |
| frontend-contract-219 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/check` | POST | checkFirewallRules | Firewall.CheckResponse | not-run |
| frontend-contract-220 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/delete` | POST | deleteFirewallRules | Firewall.DeleteResponse | not-run |
| frontend-contract-221 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/native/detail` | POST | loadFirewallNativeDetail | string | not-run |
| frontend-contract-222 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/reset` | POST | resetFirewallRules | Firewall.ResetResponse | not-run |
| frontend-contract-223 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/search` | POST | searchFirewallRules | Firewall.Inventory | not-run |
| frontend-contract-224 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/sync` | POST | syncFirewallRules | Firewall.RuleSyncResult | not-run |
| frontend-contract-225 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/sync/preview` | POST | previewFirewallRuleSync | Firewall.RuleSyncPreview | not-run |
| frontend-contract-226 | menu.system | HTTP | `/api/v2/hosts/firewall/rules/sync/task` | GET | loadFirewallRuleSyncTask | Firewall.RuleSyncTask | not-run |
| frontend-contract-227 | menu.system | HTTP | `/api/v2/hosts/firewall/settings` | GET | loadFirewallSettings | Firewall.Settings | not-run |
| frontend-contract-228 | menu.system | HTTP | `/api/v2/hosts/info` | POSTLOCALNODE | — | — | not-run |
| frontend-contract-229 | menu.system | HTTP | `/api/v2/hosts/monitor/setting` | GET | loadMonitorSetting | Host.MonitorSetting | not-run |
| frontend-contract-230 | menu.system | HTTP | `/api/v2/hosts/ssh/file` | POST | loadSSHFile | string | not-run |
| frontend-contract-231 | menu.system | HTTP | `/api/v2/hosts/ssh/log/export` | POST | exportSSHLogs | string | not-run |
| frontend-contract-232 | menu.system | HTTP | `/api/v2/hosts/ssh/search` | POST | getSSHInfo | Host.SSHInfo | not-run |
| frontend-contract-233 | menu.system | HTTP | `/api/v2/hosts/terminal/container` | GET | — | — | not-run |
| frontend-contract-234 | menu.system | HTTP | `/api/v2/hosts/terminal/local` | GET | — | — | not-run |
| frontend-contract-235 | menu.system | HTTP | `/api/v2/hosts/terminal/ssh` | GET | — | — | not-run |
| frontend-contract-236 | menu.system | HTTP | `/api/v2/hosts/test/byid` | POSTLOCALNODE | — | — | not-run |
| frontend-contract-237 | menu.system | HTTP | `/api/v2/hosts/test/byinfo` | POSTLOCALNODE | — | — | not-run |
| frontend-contract-238 | menu.system | HTTP | `/api/v2/hosts/tool/config/get` | POST | getSupervisorConfig | HostTool.SupervisorConfigRes | not-run |
| frontend-contract-239 | menu.system | HTTP | `/api/v2/hosts/tool/config/set` | POST | updateSupervisorConfig | any | not-run |
| frontend-contract-240 | menu.system | HTTP | `/api/v2/hosts/tool/init` | POST | initSupervisor | any | not-run |
| frontend-contract-241 | menu.system | HTTP | `/api/v2/hosts/tool/operate` | POST | operateSupervisor | any | not-run |
| frontend-contract-242 | menu.system | HTTP | `/api/v2/hosts/tool/status` | POST | getSupervisorStatus | HostTool.HostTool | not-run |
| frontend-contract-243 | menu.system | HTTP | `/api/v2/hosts/tool/supervisor/process` | POST, GET | createSupervisorProcess | any | not-run |
| frontend-contract-244 | menu.system | HTTP | `/api/v2/hosts/tool/supervisor/process/file` | POST | operateSupervisorProcessFile | any | not-run |
| frontend-contract-245 | menu.system | HTTP | `/api/v2/hosts/tool/supervisor/process/file/get` | POST | getSupervisorProcessFile | any | not-run |
| frontend-contract-246 | menu.home | HTTP | `/api/v2/images/favicon?t=:param` | GET | — | — | not-run |
| frontend-contract-247 | menu.home | HTTP | `/api/v2/images/loginBackground?t=:param` | GET | — | — | not-run |
| frontend-contract-248 | menu.home | HTTP | `/api/v2/images/loginImage?t=:param` | GET | — | — | not-run |
| frontend-contract-249 | menu.home | HTTP | `/api/v2/images/logo?t=:param` | GET | — | — | not-run |
| frontend-contract-250 | menu.home | HTTP | `/api/v2/images/logoWithText?t=:param` | GET | — | — | not-run |
| frontend-contract-251 | menu.logs | HTTP | `/api/v2/logs/system/read:param` | POST | readSystemLogs | Log.SystemLog | not-run |
| frontend-contract-252 | menu.logs | HTTP | `/api/v2/logs/system/services:param` | GET | listRunningServices | string[] | not-run |
| frontend-contract-253 | menu.logs | HTTP | `/api/v2/logs/system/status:param` | GET | getSystemLogStatus | Log.SystemLogStatus | not-run |
| frontend-contract-254 | menu.logs | HTTP | `/api/v2/logs/tasks/executing/count` | GET | countExecutingTask | number | not-run |
| frontend-contract-255 | menu.logs | HTTP | `/api/v2/logs/tasks/read:param` | POST | readTaskLogByLine | any | not-run |
| frontend-contract-256 | menu.website | HTTP | `/api/v2/openresty` | GET | getNginx | File.File | not-run |
| frontend-contract-257 | menu.website | HTTP | `/api/v2/openresty/https` | GET | getHttpsStatus | Nginx.NginxHttpsStatus | not-run |
| frontend-contract-258 | menu.website | HTTP | `/api/v2/openresty/modules` | GET | getNginxModules | Nginx.NginxBuildConfig | not-run |
| frontend-contract-259 | menu.website | HTTP | `/api/v2/openresty/scope` | POST | getNginxConfigByScope | Nginx.NginxParam[] | not-run |
| frontend-contract-260 | menu.website | HTTP | `/api/v2/openresty/status` | GET | getNginxStatus | Nginx.NginxStatus | not-run |
| frontend-contract-261 | menu.home | HTTP | `/api/v2/process/:param` | GET | getProcessByID | Process.PsProcessData | not-run |
| frontend-contract-262 | menu.home | HTTP | `/api/v2/process/listening` | POST | getListeningProcess | Process.ListeningProcess[] | not-run |
| frontend-contract-263 | menu.home | HTTP | `/api/v2/process/stop` | POST | stopProcess | any | not-run |
| frontend-contract-264 | menu.home | HTTP | `/api/v2/runtimes` | POST | CreateRuntime | Runtime.Runtime | not-run |
| frontend-contract-265 | menu.home | HTTP | `/api/v2/runtimes/:param` | GET | GetRuntime | Runtime.RuntimeDTO | not-run |
| frontend-contract-266 | menu.home | HTTP | `/api/v2/runtimes/del` | POST | DeleteRuntime | any | not-run |
| frontend-contract-267 | menu.home | HTTP | `/api/v2/runtimes/installed/delete/check/:param` | GET | RuntimeDeleteCheck | App.AppInstallResource[] | not-run |
| frontend-contract-268 | menu.home | HTTP | `/api/v2/runtimes/node/modules` | POST | GetNodeModules | Runtime.NodeModule[] | not-run |
| frontend-contract-269 | menu.home | HTTP | `/api/v2/runtimes/node/modules/operate` | POST | OperateNodeModule | any | not-run |
| frontend-contract-270 | menu.home | HTTP | `/api/v2/runtimes/node/package` | POST | GetNodeScripts | Runtime.NodeScripts[] | not-run |
| frontend-contract-271 | menu.home | HTTP | `/api/v2/runtimes/operate` | POST | OperateRuntime | any | not-run |
| frontend-contract-272 | menu.home | HTTP | `/api/v2/runtimes/php/:param/extensions` | GET | GetPHPExtensions | Runtime.PHPExtensionsRes | not-run |
| frontend-contract-273 | menu.home | HTTP | `/api/v2/runtimes/php/config` | POST | UpdatePHPConfig | any | not-run |
| frontend-contract-274 | menu.home | HTTP | `/api/v2/runtimes/php/config/:param` | GET | GetPHPConfig | Runtime.PHPConfig | not-run |
| frontend-contract-275 | menu.home | HTTP | `/api/v2/runtimes/php/container/:param` | GET | getPHPContainerConfig | Runtime.PHPContainerConfig | not-run |
| frontend-contract-276 | menu.home | HTTP | `/api/v2/runtimes/php/container/update` | POST | updatePHPContainerConfig | any | not-run |
| frontend-contract-277 | menu.home | HTTP | `/api/v2/runtimes/php/extensions` | POST | CreatePHPExtensions | any | not-run |
| frontend-contract-278 | menu.home | HTTP | `/api/v2/runtimes/php/extensions/del` | POST | DeletePHPExtensions | any | not-run |
| frontend-contract-279 | menu.home | HTTP | `/api/v2/runtimes/php/extensions/update` | POST | UpdatePHPExtensions | any | not-run |
| frontend-contract-280 | menu.home | HTTP | `/api/v2/runtimes/php/file` | POST | GetPHPConfigFile | File.File | not-run |
| frontend-contract-281 | menu.home | HTTP | `/api/v2/runtimes/php/fpm/config/:param` | GET | GetFPMConfig | Runtime.FPMConfig | not-run |
| frontend-contract-282 | menu.home | HTTP | `/api/v2/runtimes/php/fpm/status/:param` | GET | getFPMStatus | Runtime.FpmStatus[] | not-run |
| frontend-contract-283 | menu.home | HTTP | `/api/v2/runtimes/php/update` | POST | UpdatePHPFile | any | not-run |
| frontend-contract-284 | menu.home | HTTP | `/api/v2/runtimes/supervisor/process/:param` | GET | GetSupervisorProcess | HostTool.ProcessStatus[] | not-run |
| frontend-contract-285 | menu.home | HTTP | `/api/v2/runtimes/supervisor/process/file` | POST | operateSupervisorProcessFile | string | not-run |
| frontend-contract-286 | menu.home | HTTP | `/api/v2/runtimes/update` | POST | UpdateRuntime | any | not-run |
| frontend-contract-287 | menu.settings | HTTP | `/api/v2/settings/basedir:param` | GET | loadBaseDir | string | not-run |
| frontend-contract-288 | menu.settings | HTTP | `/api/v2/settings/daemonjson` | GET | loadDaemonJsonPath | string | not-run |
| frontend-contract-289 | menu.settings | HTTP | `/api/v2/settings/file-history/search` | POST | getAgentFileHistoryInfo | Setting.FileHistoryInfo | not-run |
| frontend-contract-290 | menu.settings | HTTP | `/api/v2/settings/files/ai/search` | POST | getAgentFileManageAIInfo | Setting.FileManageAIInfo | not-run |
| frontend-contract-291 | menu.settings | HTTP | `/api/v2/settings/search` | POST | getAgentSettingInfo | Setting.AgentSettingInfo | not-run |
| frontend-contract-292 | menu.settings | HTTP | `/api/v2/settings/snapshot/load` | GET | loadSnapshotInfo | Setting.SnapshotData | not-run |
| frontend-contract-293 | menu.settings | HTTP | `/api/v2/settings/ssh/check` | POST | testLocalConn | boolean | not-run |
| frontend-contract-294 | menu.settings | HTTP | `/api/v2/settings/ssh/check/info` | POST | testByInfo | boolean | not-run |
| frontend-contract-295 | menu.settings | HTTP | `/api/v2/settings/ssh/conn` | GET | loadLocalConn | Host.HostConnTest | not-run |
| frontend-contract-296 | menu.settings | HTTP | `/api/v2/settings/terminal/ai/search` | POST | getAgentTerminalAIInfo | Setting.TerminalAIInfo | not-run |
| frontend-contract-297 | menu.settings | HTTP | `/api/v2/settings/website/dir` | GET | loadWebsiteDir | string | not-run |
| frontend-contract-298 | menu.toolbox | HTTP | `/api/v2/toolbox/clam/base` | POST | searchClamBaseInfo | Toolbox.ClamBaseInfo | not-run |
| frontend-contract-299 | menu.toolbox | HTTP | `/api/v2/toolbox/clam/file/search` | POST | searchClamFile | string | not-run |
| frontend-contract-300 | menu.toolbox | HTTP | `/api/v2/toolbox/device/base` | POST | getDeviceBase | Toolbox.DeviceBaseInfo | not-run |
| frontend-contract-301 | menu.toolbox | HTTP | `/api/v2/toolbox/fail2ban/base` | GET | getFail2banBase | Toolbox.Fail2banBaseInfo | not-run |
| frontend-contract-302 | menu.toolbox | HTTP | `/api/v2/toolbox/fail2ban/load/conf` | GET | getFail2banConf | string | not-run |
| frontend-contract-303 | menu.toolbox | HTTP | `/api/v2/toolbox/ftp/base` | GET | getFtpBase | Toolbox.FtpBaseInfo | not-run |
| frontend-contract-304 | menu.toolbox | HTTP | `/api/v2/toolbox/scan` | POST | scan | Toolbox.CleanData | not-run |
| frontend-contract-305 | menu.website | HTTP | `/api/v2/websites` | POST | createWebsite | any | not-run |
| frontend-contract-306 | menu.website | HTTP | `/api/v2/websites/:param` | GET | getWebsite | Website.WebsiteDTO | not-run |
| frontend-contract-307 | menu.website | HTTP | `/api/v2/websites/:param/config/:param` | GET | getWebsiteConfig | File.File | not-run |
| frontend-contract-308 | menu.website | HTTP | `/api/v2/websites/:param/https` | GET, POST | getHTTPSConfig | Website.HTTPSConfig | not-run |
| frontend-contract-309 | menu.website | HTTP | `/api/v2/websites/:param/lbs` | GET | getLoadBalances | Website.NginxUpstream[] | not-run |
| frontend-contract-310 | menu.website | HTTP | `/api/v2/websites/acme` | POST | createAcmeAccount | Website.AcmeAccount | not-run |
| frontend-contract-311 | menu.website | HTTP | `/api/v2/websites/acme/del` | POST | deleteAcmeAccount | any | not-run |
| frontend-contract-312 | menu.website | HTTP | `/api/v2/websites/acme/update` | POST | updateAcmeAccount | Website.AcmeAccount | not-run |
| frontend-contract-313 | menu.website | HTTP | `/api/v2/websites/auths` | POST | getAuthConfig | Website.AuthConfig | not-run |
| frontend-contract-314 | menu.website | HTTP | `/api/v2/websites/auths/path` | POST | getPathAuthConfig | Website.NginxPathAuthConfig[] | not-run |
| frontend-contract-315 | menu.website | HTTP | `/api/v2/websites/auths/update` | POST | operateAuthConfig | any | not-run |
| frontend-contract-316 | menu.website | HTTP | `/api/v2/websites/ca` | POST | createCA | Website.CA | not-run |
| frontend-contract-317 | menu.website | HTTP | `/api/v2/websites/ca/:param` | GET | getCA | Website.CADTO | not-run |
| frontend-contract-318 | menu.website | HTTP | `/api/v2/websites/ca/del` | POST | deleteCA | any | not-run |
| frontend-contract-319 | menu.website | HTTP | `/api/v2/websites/ca/download` | DOWNLOAD | downloadCAFile | BlobPart | not-run |
| frontend-contract-320 | menu.website | HTTP | `/api/v2/websites/ca/obtain` | POST | obtainSSLByCA | any | not-run |
| frontend-contract-321 | menu.website | HTTP | `/api/v2/websites/ca/renew` | POST | renewSSLByCA | any | not-run |
| frontend-contract-322 | menu.website | HTTP | `/api/v2/websites/check` | POST | preCheck | Website.CheckRes[] | not-run |
| frontend-contract-323 | menu.website | HTTP | `/api/v2/websites/config` | POST | getNginxConfig | Website.NginxScopeConfig | not-run |
| frontend-contract-324 | menu.website | HTTP | `/api/v2/websites/config/update` | POST | updateNginxConfig | any | not-run |
| frontend-contract-325 | menu.website | HTTP | `/api/v2/websites/cors/:param` | GET | getCorsConfig | Website.CorsConfig | not-run |
| frontend-contract-326 | menu.website | HTTP | `/api/v2/websites/databases` | GET | getWebsiteDatabase | Website.WebsiteDatabase[] | not-run |
| frontend-contract-327 | menu.website | HTTP | `/api/v2/websites/default/html/:param` | GET | getDefaultHtml | Website.WebsiteHtml | not-run |
| frontend-contract-328 | menu.website | HTTP | `/api/v2/websites/default/server` | POST | changeDefaultServer | any | not-run |
| frontend-contract-329 | menu.website | HTTP | `/api/v2/websites/del` | POST | deleteWebsite | any | not-run |
| frontend-contract-330 | menu.website | HTTP | `/api/v2/websites/dir` | POST | getDirConfig | Website.DirConfig | not-run |
| frontend-contract-331 | menu.website | HTTP | `/api/v2/websites/dir/permission` | POST | updateWebsiteDirPermission | any | not-run |
| frontend-contract-332 | menu.website | HTTP | `/api/v2/websites/dir/update` | POST | updateWebsiteDir | any | not-run |
| frontend-contract-333 | menu.website | HTTP | `/api/v2/websites/dns` | POST | createDnsAccount | any | not-run |
| frontend-contract-334 | menu.website | HTTP | `/api/v2/websites/dns/del` | POST | deleteDnsAccount | any | not-run |
| frontend-contract-335 | menu.website | HTTP | `/api/v2/websites/dns/update` | POST | updateDnsAccount | any | not-run |
| frontend-contract-336 | menu.website | HTTP | `/api/v2/websites/domains` | POST | createDomain | any | not-run |
| frontend-contract-337 | menu.website | HTTP | `/api/v2/websites/domains/:param` | GET | listDomains | Website.Domain[] | not-run |
| frontend-contract-338 | menu.website | HTTP | `/api/v2/websites/domains/del/` | POST | deleteDomain | any | not-run |
| frontend-contract-339 | menu.website | HTTP | `/api/v2/websites/domains/update` | POST | updateDomain | any | not-run |
| frontend-contract-340 | menu.website | HTTP | `/api/v2/websites/leech` | POST | getAntiLeech | Website.LeechConfig | not-run |
| frontend-contract-341 | menu.website | HTTP | `/api/v2/websites/leech/update` | POST | updateAntiLeech | any | not-run |
| frontend-contract-342 | menu.website | HTTP | `/api/v2/websites/list` | GET | listWebsites | Website.WebsiteDTO[] | not-run |
| frontend-contract-343 | menu.website | HTTP | `/api/v2/websites/log/operate` | POST | opWebsiteLog | any | not-run |
| frontend-contract-344 | menu.website | HTTP | `/api/v2/websites/log/search` | POST | getWebsiteLog | Website.WebSiteLog | not-run |
| frontend-contract-345 | menu.website | HTTP | `/api/v2/websites/nginx/update` | POST | updateNginxFile | any | not-run |
| frontend-contract-346 | menu.website | HTTP | `/api/v2/websites/operate:param` | POST | opWebsite | any | not-run |
| frontend-contract-347 | menu.website | HTTP | `/api/v2/websites/options` | POST | getWebsiteOptions | Website.WebsiteOption[] | not-run |
| frontend-contract-348 | menu.website | HTTP | `/api/v2/websites/php/version` | POST | changePHPVersion | any | not-run |
| frontend-contract-349 | menu.website | HTTP | `/api/v2/websites/proxies` | POST | getProxyConfig | Website.ProxyConfig[] | not-run |
| frontend-contract-350 | menu.website | HTTP | `/api/v2/websites/proxies/delete` | POST | deleteProxyConfig | any | not-run |
| frontend-contract-351 | menu.website | HTTP | `/api/v2/websites/proxies/file` | POST | updateProxyConfigFile | any | not-run |
| frontend-contract-352 | menu.website | HTTP | `/api/v2/websites/proxies/status` | POST | updateProxyConfigStatus | any | not-run |
| frontend-contract-353 | menu.website | HTTP | `/api/v2/websites/proxies/update` | POST | operateProxyConfig | any | not-run |
| frontend-contract-354 | menu.website | HTTP | `/api/v2/websites/proxy/config/:param` | GET | getCacheConfig | Website.WebsiteCacheConfig | not-run |
| frontend-contract-355 | menu.website | HTTP | `/api/v2/websites/realip/config/:param` | GET | getRealIPConfig | Website.WebsiteRealIPConfig | not-run |
| frontend-contract-356 | menu.website | HTTP | `/api/v2/websites/redirect` | POST | getRedirectConfig | Website.RedirectConfig[] | not-run |
| frontend-contract-357 | menu.website | HTTP | `/api/v2/websites/redirect/file` | POST | updateRedirectConfigFile | any | not-run |
| frontend-contract-358 | menu.website | HTTP | `/api/v2/websites/redirect/update` | POST | operateRedirectConfig | any | not-run |
| frontend-contract-359 | menu.website | HTTP | `/api/v2/websites/resource/:param` | GET | getWebsiteResource | Website.WebsiteResource[] | not-run |
| frontend-contract-360 | menu.website | HTTP | `/api/v2/websites/rewrite` | POST | getRewriteConfig | Website.RewriteRes | not-run |
| frontend-contract-361 | menu.website | HTTP | `/api/v2/websites/rewrite/custom` | GET | operateCustomRewrite | — | not-run |
| frontend-contract-362 | menu.website | HTTP | `/api/v2/websites/rewrite/update` | POST | updateRewriteConfig | any | not-run |
| frontend-contract-363 | menu.website | HTTP | `/api/v2/websites/ssl` | POST | createSSL | Website.SSLCreate | not-run |
| frontend-contract-364 | menu.website | HTTP | `/api/v2/websites/ssl/:param` | GET | getSSL | Website.SSL | not-run |
| frontend-contract-365 | menu.website | HTTP | `/api/v2/websites/ssl/del` | POST | deleteSSL | any | not-run |
| frontend-contract-366 | menu.website | HTTP | `/api/v2/websites/ssl/download` | DOWNLOAD | downloadFile | BlobPart | not-run |
| frontend-contract-367 | menu.website | HTTP | `/api/v2/websites/ssl/list` | POST, POSTLOCALNODE | listSSL | Website.SSLDTO[] | not-run |
| frontend-contract-368 | menu.website | HTTP | `/api/v2/websites/ssl/obtain` | POST | obtainSSL | any | not-run |
| frontend-contract-369 | menu.website | HTTP | `/api/v2/websites/ssl/push` | POST | pushSSLToNode | any | not-run |
| frontend-contract-370 | menu.website | HTTP | `/api/v2/websites/ssl/resolve` | POST | getDnsResolve | Website.DNSResolve[] | not-run |
| frontend-contract-371 | menu.website | HTTP | `/api/v2/websites/ssl/update` | POST | updateSSL | any | not-run |
| frontend-contract-372 | menu.website | HTTP | `/api/v2/websites/ssl/upload` | POST | uploadSSL | any | not-run |
| frontend-contract-373 | menu.website | HTTP | `/api/v2/websites/ssl/upload/file` | UPLOAD | uploadSSLFile | File.File | not-run |
| frontend-contract-374 | menu.website | HTTP | `/api/v2/websites/templates` | POST | createTemplate | any | not-run |
| frontend-contract-375 | menu.website | HTTP | `/api/v2/websites/templates/del` | POST | deleteTemplate | any | not-run |
| frontend-contract-376 | menu.website | HTTP | `/api/v2/websites/templates/get` | POST | getTemplate | Website.Template | not-run |
| frontend-contract-377 | menu.website | HTTP | `/api/v2/websites/templates/outputs` | POST | createTemplateOutput | any | not-run |
| frontend-contract-378 | menu.website | HTTP | `/api/v2/websites/templates/outputs/del` | POST | deleteTemplateOutput | any | not-run |
| frontend-contract-379 | menu.website | HTTP | `/api/v2/websites/templates/outputs/get` | POST | getTemplateOutput | Website.TemplateOutputDTO | not-run |
| frontend-contract-380 | menu.website | HTTP | `/api/v2/websites/templates/preview` | POST | previewTemplate | Website.PreviewDTO | not-run |
| frontend-contract-381 | menu.website | HTTP | `/api/v2/websites/templates/update` | POST | updateTemplate | any | not-run |
| frontend-contract-382 | menu.website | HTTP | `/api/v2/websites/templates/upload` | POST | uploadTemplateZip | { filePath: string; variables: string[] } | not-run |
| frontend-contract-383 | menu.website | HTTP | `/api/v2/websites/update:param` | POST | updateWebsite | any | not-run |
| frontend-contract-384 | menu.home | HTTP | `/api/v2/xpack/alert/logs/sync` | POST | SyncAlertInfo | any | not-run |
| frontend-contract-385 | menu.home | HTTP | `/api/v2/xpack/alert/logs/sync/all` | POST | SyncAlertAll | any | not-run |
| frontend-contract-386 | menu.home | WS | `/api/v2/core/script/run` | WS | — | — | not-run |
| frontend-contract-387 | menu.system | WS | `/api/v2/hosts/terminal/container` | WS | — | — | not-run |
| frontend-contract-388 | menu.system | WS | `/api/v2/hosts/terminal/local` | WS | — | — | not-run |
| frontend-contract-389 | menu.system | WS | `/api/v2/hosts/terminal/ssh` | WS | — | — | not-run |

完整字段、请求函数签名、调用摘要、参数维度、响应接口文件、菜单映射和执行证据占位符请查看同目录 JSON。

## 执行规则

1. 先使用有效登录会话和真实 SQLite，不得使用固定成功、空数组或 JSON 模拟业务数据。
2. 按菜单分组执行 HTTP/WS；同一接口的 method、路径参数、`type`、`operate`、`logType`、`source`、`scope` 必须分别记录。
3. 每条案例必须保存脱敏请求摘要、HTTP 状态码、响应 envelope、关键字段类型、任务 ID/WS 消息序列、SQLite 或外部副作用。
4. 只有真实执行器生成独立证据后，才能把案例状态从 `not-run` 改成 `pass`、`fail` 或 `blocked`；本生成器不会覆盖测试状态。
5. 上传、下载、SSE、终端 PTY、断线释放和重连等流式能力要记录连接建立、消息顺序、关闭码和资源释放。

## 当前静态限制

- 请求参数字段来自当前 API 模块函数签名和调用表达式；不能自动生成合法业务样例。
- 响应字段来源只记录当前泛型和接口文件；后端实际 envelope、错误分支和动态字段必须真实请求验证。
- WS 的 4 条调用必须追加消息协议、心跳、断线、权限和资源释放测试，不能只验证握手成功。
