import { defineConfig } from "vitepress";

// GitHub Pages project site: https://<org>.github.io/<repo>/
export default defineConfig({
  title: "OneSSH",
  description:
    "CLI SSH manager with a single master password and encrypted configuration.",
  base: "/onessh/",
  cleanUrls: true,

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

    socialLinks: [
      { icon: "github", link: "https://github.com/tiangong-dev/onessh" },
    ],

    footer: {
      message: "Released under the Unlicense.",
    },

    search: {
      provider: "local",
    },
  },
});
