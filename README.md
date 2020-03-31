# Syncthing Tech UI

## Development server

Run `npm run serve` for a dev server. Navigate to `http://localhost:4200/`. The
app will automatically reload if you change any of the source files.

## Production server

In production we serve the UI through Syncthing itself. The easiest way to
do that is to simply put the built assets in the `gui` subdirectory of
Syncthing's config directory.

- `npm run build -- --prod`
- `rsync -va --delete dist/tech-ui/ ~/.config/syncthing/gui/default/tech-ui/`

Adjust for your actual Syncthing config dir if different. Navigate to
`http://localhost:8384/tech-ui/`.

## Code scaffolding

Run `ng generate component component-name` to generate a new component. You
can also use `ng generate
directive|pipe|service|class|guard|interface|enum|module`.

# License

MIT
