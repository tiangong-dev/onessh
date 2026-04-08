import { defineConfig } from "vitepress";

// GitHub Pages project site: https://<org>.github.io/<repo>/
export default defineConfig({
  title: "OneSSH",
  description:
    "CLI SSH manager with a single master password and encrypted configuration.",
  base: "/onessh/",
  cleanUrls: true,

  locales: {
    root: {
      label: "English",
      lang: "en",
      themeConfig: {
        nav: [
          { text: "Guide", link: "/guide/getting-started" },
          {
            text: "Reference",
            items: [
              { text: "Overview", link: "/reference/" },
              { text: "Architecture", link: "/reference/architecture" },
              { text: "Security", link: "/reference/security" },
            ],
          },
          {
            text: "Repository",
            link: "https://github.com/tiangong-dev/onessh",
          },
        ],

        sidebar: {
          "/guide/": [
            {
              text: "Guide",
              items: [
                { text: "Getting started", link: "/guide/getting-started" },
                { text: "Commands", link: "/guide/commands" },
                { text: "Configuration", link: "/guide/configuration" },
              ],
            },
          ],
          "/reference/": [
            {
              text: "Reference",
              items: [
                { text: "Overview", link: "/reference/" },
                { text: "Architecture", link: "/reference/architecture" },
                { text: "Security", link: "/reference/security" },
              ],
            },
          ],
        },

        footer: {
          message: "Released under the Unlicense.",
        },
      },
    },

    zh: {
      label: "简体中文",
      lang: "zh-CN",
      link: "/zh/",
      themeConfig: {
        nav: [
          { text: "指南", link: "/zh/guide/getting-started" },
          {
            text: "参考",
            items: [
              { text: "概览", link: "/zh/reference/" },
              { text: "架构", link: "/zh/reference/architecture" },
              { text: "安全", link: "/zh/reference/security" },
            ],
          },
          {
            text: "仓库",
            link: "https://github.com/tiangong-dev/onessh",
          },
        ],

        sidebar: {
          "/zh/guide/": [
            {
              text: "指南",
              items: [
                { text: "快速开始", link: "/zh/guide/getting-started" },
                { text: "命令", link: "/zh/guide/commands" },
                { text: "配置", link: "/zh/guide/configuration" },
              ],
            },
          ],
          "/zh/reference/": [
            {
              text: "参考",
              items: [
                { text: "概览", link: "/zh/reference/" },
                { text: "架构", link: "/zh/reference/architecture" },
                { text: "安全", link: "/zh/reference/security" },
              ],
            },
          ],
        },

        footer: {
          message: "基于 Unlicense 发布。",
        },
      },
    },
  },

  themeConfig: {
    socialLinks: [
      { icon: "github", link: "https://github.com/tiangong-dev/onessh" },
    ],

    search: {
      provider: "local",
      options: {
        locales: {
          root: {
            translations: {
              button: {
                buttonText: "Search",
                buttonAriaLabel: "Search docs",
              },
              modal: {
                displayDetails: "Display detailed list",
                resetButtonTitle: "Reset search",
                backButtonTitle: "Close search",
                noResultsText: "No results for",
                footer: {
                  selectText: "to select",
                  navigateText: "to navigate",
                  closeText: "to close",
                },
              },
            },
          },
          zh: {
            translations: {
              button: {
                buttonText: "搜索",
                buttonAriaLabel: "搜索文档",
              },
              modal: {
                displayDetails: "显示详细列表",
                resetButtonTitle: "清除搜索条件",
                backButtonTitle: "关闭搜索",
                noResultsText: "未找到相关结果",
                footer: {
                  selectText: "选择",
                  navigateText: "切换",
                  closeText: "关闭",
                },
              },
            },
          },
        },
      },
    },
  },
});
