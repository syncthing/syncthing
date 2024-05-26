name: Bug report
description: If you're actually looking for support instead, see "I need help / I have a question".
labels: ["bug", "needs-triage"]
body:
  - type: markdown
    attributes:
      value: |
        :no_entry_sign: If you want to report a security issue, please see [our Security Policy](https://syncthing.net/security/) and do not report the issue here.

        :interrobang: If you are not sure if there is a bug, but something isn't working right and you need help, please [use the forum](https://forum.syncthing.net/).

  - type: textarea
    id: what-happened
    attributes:
      label: What happened?
      description: Also tell us, what did you expect to happen, and any steps we might use to reproduce the problem.
      placeholder: Tell us what you see!
    validations:
      required: true

  - type: input
    id: version
    attributes:
      label: Syncthing version
      description: What version of Syncthing are you running?
      placeholder: v1.27.4
    validations:
      required: true

  - type: input
    id: platform
    attributes:
      label: Platform & operating system
      description: On what platform(s) are you seeing the problem?
      placeholder: Linux arm64
    validations:
      required: true

  - type: input
    id: browser
    attributes:
      label: Browser version
      description: If the problem is related to the GUI, describe your browser and version.
      placeholder: Safari 17.3.1

  - type: textarea
    id: logs
    attributes:
      label: Relevant log output
      description: Please copy and paste any relevant log output or crash backtrace. This will be automatically formatted into code, so no need for backticks.
      render: shell
