describe('IgnoreTreeService', function() {
    beforeEach(module('syncthing.folder'));

    var service, BrowseService, IgnoresService;
    var browseSpy;

    beforeEach(inject(function($injector) {
        service = $injector.get('IgnoreTree');
        BrowseService = $injector.get('Browse');
        IgnoresService = $injector.get('Ignores');
    }));
    beforeEach(setup);
    afterEach(teardown);

    function setup() {
        browseSpy = spyOn(BrowseService, 'refresh').and.returnValue(Promise.resolve({ files: [
            { name: 'Backups', path: 'Backups', isFile: false },
            { name: 'file.txt', path: 'file.txt', isFile: true },
            { name: 'Photos', path: 'Photos', isFile: false },
        ] }));
        // Prepare a table for fancytree to render into
        var table = $('<table>', { id: 'ignore-tree' }).append(
            $('<thead>').append(
                $('<tr>').append(
                    $('<th>'),
                    $('<th>'),
                )
            )
        );
        $('body').append(table);
    }

    function teardown() {
        $('table#ignore-tree').remove();
    }

    async function loadTable(folderId) {
        service.refresh(folderId);
        // Not sure why two "ticks" are needed for table to resolve and render
        await Promise.resolve();
        await Promise.resolve();
    }

    describe('refresh', function() {
        beforeEach(() => IgnoresService.addPattern('/file.txt'));

        it('renders a table row for each file', async function () {
            await loadTable('default');
            expect($('table tbody tr').map((i, el) => $(el).text()).get()).toEqual(['Backups', 'file.txt', 'Photos']);
        });

        it('renders ignored files unchecked', async function () {
            await loadTable('default');
            // fancytree renders its checkboxes with a styled
            // span[role=checkbox], not a real checkbox input. I think finding
            // this class is the best we can do to test checked state :(
            expect($('table tbody tr.fancytree-selected').map((i, el) => $(el).text()).get()).toEqual(['Backups', 'Photos']);
        });

        it('reloads contents when tree exists', async function () {
            await loadTable('default');
            browseSpy.and.returnValue(Promise.resolve({ files: [{ name: 'Radiohead', path: 'Radiohead', isFile: false }] }));
            await loadTable('music');
            expect($('table tbody tr').map((i, el) => $(el).text()).get()).toEqual(['Radiohead']);
            $('table tbody tr .fancytree-expander').click();
            expect(browseSpy).toHaveBeenCalledWith('music', 'Radiohead')
        });
    });

    describe('update', function() {
        it('updates with new match content', async function() {
            await loadTable('default');
            expect($('tr.fancytree-selected:contains(Backups)').length).toEqual(1);
            IgnoresService.addPattern('/Backups')
            service.update();
            expect($('tr.fancytree-selected:contains(Backups)').length).toEqual(0);
        });

        it('does not fail when tree is not rendered', function() {
            service.update();
        });
    });
});
