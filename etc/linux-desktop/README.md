# Desktop Entries

This directory contains files to integrate Syncthing in your desktop environment (DE).
Specifically this works for DEs that implement the [XDG Desktop Menu Specification][1], which
is virtually every DE.  
To add Syncthing to desktop menus for all users, copy the `.desktop` files to
`/usr/local/share/applications` (root required). To add it for just your user, copy them to `~/.local/share/applications`.  
To start Syncthing automatically, you have two options: Either you go to the autostart settings of your DE and choose Syncthing or you copy the `syncthing-start.desktop` file to `~/.config/autostart`.  
For more information refer to the [ArchWiki page on Desktop entries][2]

[1]: https://specifications.freedesktop.org/menu-spec/menu-spec-latest.html
[2]: https://wiki.archlinux.org/index.php/Desktop_entries
