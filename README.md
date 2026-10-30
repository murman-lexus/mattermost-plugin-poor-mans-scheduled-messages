# Poor Man's Scheduled Messages

[![Build Status](https://github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/actions/workflows/ci.yml/badge.svg)](https://github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/actions/workflows/ci.yml)
[![CodeQL](https://github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/actions/workflows/github-code-scanning/codeql)

<div align="center">
  <img src="logo.png" alt="Poor Man's Scheduled Messages logo">
</div>

## Purpose

The paid version of [Mattermost](https://mattermost.com) comes with [scheduled messages](https://docs.mattermost.com/collaborate/schedule-messages.html), but what about us poor schlubs on the free plan??

The Poor Man's Scheduled Messages plugin aims to fill this gap, albeit less elegantly. What you get is a `/schedule` [slash command](https://docs.mattermost.com/collaborate/run-slash-commands.html) that allows you to schedule text-only messages to be posted at a future time to any channel or direct message.

## Installation

Until an official release is available, the latest release tarball can be installed...

via [System Console](https://developers.mattermost.com/integrate/plugins/components/server/hello-world/#install-the-plugin)

...or...

via [mmctl](https://docs.mattermost.com/manage/mmctl-command-line-tool.html#mmctl-plugin-add)

## Usage

With the plugin installed: `/schedule help`

..or view the help [here](assets/help.md)

### Language and time display

Dates, month names, confirmations, list labels, and action buttons follow the user's Mattermost interface language. The clock format follows the user's personal time preference independently of language: `13:10` for 24-hour time or `1:10 PM` for 12-hour time. The `AM` and `PM` markers remain in English for every language.

Current preferences are read when a response is generated, including when listing existing scheduled messages. Dates use the time zone saved with the scheduled message. If settings cannot be loaded, the plugin falls back to the server's default client language (or English) and a 12-hour clock for the unavailable settings.

Error localization is partial: surrounding labels are translated, while some validation reasons (such as message count and size limits), parser details, and external technical errors remain in English. A response can therefore contain both translated and English text. Command syntax, autocomplete descriptions, and the help document also remain in English.

## Caveats

You get what you pay for, so...

1. **No attachments:** You can't attach anything to a scheduled message *(slash commands don't pass attachment data as far as I can tell)*.
2. **Send order:** Messages scheduled for the same channel or direct message at the exact same time are *NOT* guaranteed to be posted in the order they were scheduled.
3. **Message limits:**
   * 1000 scheduled messages per user
   * 50KB per message *(max message length in Mattermost interface is currently about 16KB, so shouldn't be a problem)*.
4. **High performance? Who knows:**
   * Messages are managed via Mattermost's internal key/value store.
   * A scheduler cycles through all scheduled messages once per minute, sending those that are due.
   * If you don't exceed the 'official' free plan limit of fifty users, and your users aren't all scheduling hundreds of messages, it will *probably* be fine.
