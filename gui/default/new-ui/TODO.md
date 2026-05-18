# Syncthing Svelte UI — TODO & Anforderungen

## Ziel
1:1 Feature- und Layout-Klon der AngularJS UI in Svelte 5. Gleiche Funktionalität, gleiches Aussehen, dramatisch bessere Performance (53k AngularJS Watchers → 0).

## Anforderungen / Erwartungen
- **Pixel-perfekte Übereinstimmung** mit der Original-UI
- **HTML/CSS-Gerüst identisch** zum Original — gleiche Bootstrap-Klassen, gleiche Struktur, nur die Technik darunter ist Svelte statt AngularJS
- **CSS vom Original 1:1 übernehmen** — keine eigenen CSS-Overrides wenn es vermeidbar ist. Nur Svelte-spezifisches CSS (identicon SVG, Modal-Backdrop, Login-Form)
- **Feature-Complete** — jedes Feld, jeder Dialog, jeder Button, jeder Tooltip, jede Info-Box
- **i18n komplett** — alle Strings übersetzt, Language-Selector, localStorage-Persistenz
- **Keine schreibenden Aktionen beim Testen** — nur Dialoge öffnen, nie speichern

## Strategie: CDP Snapshot Diff
1. Alte UI: targetId im Browser identifizieren
2. Neue UI: targetId im Browser identifizieren
3. `cdp_snapshot` auf beiden → Accessibility-Tree-Text vergleichen
4. Unterschiede in Struktur/Text → im Svelte-Code fixen
5. Build → Deploy → Reload neue UI → erneut vergleichen
6. Wiederholen bis identisch

### Hover-Tooltips testen
- `cdp_hover` auf Element → `cdp_snapshot` → Tooltip-Inhalt/Position prüfen
- Vergleichen mit Original

### Keine Screenshots verwenden
- `cdp_snapshot` (DOM/Accessibility Tree) bevorzugen — leichtgewichtiger, vergleichbar
- Screenshots nur als letztes Mittel wenn visuelles Layout nicht anders prüfbar

## Offene Bugs

### Kritisch (funktional)
- [ ] 19. Download/Upload Rate aktualisiert sporadisch — Events/Polling prüfen, `/rest/system/connections` Intervall vergleichen mit Original
- [ ] 20. Versions Button — Overlay/UI fehlt oder öffnet nicht

### Layout / Visuell
- [ ] 15. Folder-Text bei Devices — CSS jetzt gefixt (overflow-break-word), verifizieren
- [ ] 16. Device Icon Spacing — gefixt (getrennte panel-icon/identicon Spans), verifizieren
- [ ] 17. Hover-Tooltips — Positionierung/Inhalt per synthetic hover testen
- [ ] 18. Discovery Link — im "This Device" Block fehlt die Verlinkung zum Discovery-Status-Overlay

### Feature-Completeness
- [ ] 6. Info-Text-Boxen — Agent hat viele ergänzt, verifizieren ob alle da sind
- [ ] 7. Fehlende Input-Felder — Agent hat ergänzt, verifizieren
- [ ] 8. Pagination — volle Seitennavigation (1..N), Agent hat implementiert, verifizieren
- [ ] 9. Out of Sync Items — Ladebalken + Legende, Agent hat implementiert, verifizieren
- [ ] 10/13. Tooltips — Custom Bootstrap-Style Tooltips, Agent hat tooltip.js erstellt, verifizieren
- [ ] 11. Rescan Interval-Anzeige — Agent hat ergänzt, verifizieren
- [ ] 14. Page-Size Selection — Agent hat implementiert, verifizieren

### Abgeschlossen
- [x] 1. Instance-Name in Header — reaktiv via $derived.by()
- [x] 2. Language Selection — i18n System komplett eingebaut
- [x] 3. Identicons — exakte Port der Original-Logik + fill:currentColor
- [x] 4. Folder-Edit 2-Spalten-Layout — Agent hat umgebaut
- [x] 5. Layout-Mismatches — Agent hat viele gefixt

### Systematischer Abgleich
- [ ] 12. Angular ↔ Svelte Zeile-für-Zeile Vergleich — per CDP Snapshot Diff
  - [ ] Main page (Folders collapsed)
  - [ ] Main page (Folder expanded)
  - [ ] Main page (Device expanded)
  - [ ] This Device expanded
  - [ ] Folder Edit Dialog (alle Tabs)
  - [ ] Device Edit Dialog (alle Tabs)
  - [ ] Settings Dialog (alle Tabs)
  - [ ] About Dialog
  - [ ] Log Viewer
  - [ ] Need Files / Out of Sync
  - [ ] Failed Files
  - [ ] Global Changes / Recent Changes
  - [ ] Remote Need Files

## Deploy-Prozess
```bash
# Build
cd ~/Projects/syncthing/gui/default/new-ui && npm run build

# Deploy Nexus
scp -r dist/* frederic@nexus:/tmp/new-ui/
ssh frederic@nexus "sudo cp -r /tmp/new-ui/* /etc/syncthing/gui/default/new-ui/"

# Deploy Master
scp -r dist/* master:/etc/syncthing/gui/default/new-ui/

# Reload nur die neue UI im Browser (nicht die alte!)
```

## Architektur
- **Svelte 5** mit Runes ($state, $derived, $effect)
- **Vite 8** als Build-Tool
- **Kein SvelteKit** — Backend existiert bereits (Syncthing REST API)
- **Bootstrap CSS** vom Original übernommen (kein eigenes CSS-Framework)
- **Stores** für shared State (Svelte writable/derived stores)
- **Events** via Long-Polling `/rest/events?since=N`
- **i18n** via eigenes Modul das Language-JSON von `/assets/lang/` lädt

## Dateien
```
gui/default/new-ui/
├── package.json
├── vite.config.js
├── index.html
├── src/
│   ├── main.js
│   ├── App.svelte
│   ├── lib/
│   │   ├── api.js        — REST API Client
│   │   ├── events.js     — Long-Polling Event Handler
│   │   ├── stores.js     — Svelte Stores für shared State
│   │   ├── utils.js      — Formatter, Status-Berechnung, Identicons
│   │   ├── i18n.js       — Internationalisierung
│   │   └── tooltip.js    — Custom Tooltip Svelte Action
│   └── components/
│       ├── Header.svelte
│       ├── FolderList.svelte / FolderItem.svelte
│       ├── DeviceList.svelte / DeviceItem.svelte
│       ├── ThisDevice.svelte
│       ├── LoginForm.svelte / Modal.svelte / Notifications.svelte
│       └── modals/ (12 Dateien)
└── dist/                  — Build Output
```
