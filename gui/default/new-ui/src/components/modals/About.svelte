<script>
  import { onMount } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { version, system, onclose } = $props();
  let paths = $state({});
  let activeTab = $state('authors');
  let authors = $state('');
  let includedSW = $state('');

  onMount(async () => {
    try {
      paths = await api.getSystemPaths();
    } catch (e) {
      console.error('Error loading paths:', e);
    }
  });

  function buildDate() {
    if (!version?.date) return '';
    try {
      const d = new Date(version.date);
      return d.toISOString().split('T')[0];
    } catch (e) {
      return version.date;
    }
  }

  function upgradeTag() {
    if (version?.tags?.includes('noupgrade')) return '(noupgrade)';
    return '';
  }
</script>

<Modal title={t('About')} status="info" icon="fas fa-heart" {onclose}>
  <div class="modal-body">
    <div class="text-center" style="margin-bottom: 20px;">
      <img src="/assets/img/logo-horizontal.svg" alt="Syncthing" style="max-height: 80px;" />
    </div>

    <p class="text-center text-muted" style="font-size: 1.5em;">
      {utils.versionString(version)}
    </p>

    {#if version?.codename}
      <p class="text-center" style="font-size: 1.2em; font-style: italic;">
        "{version.codename}"
      </p>
    {/if}

    <p class="text-center text-muted">
      {$translations, t('Build')} {buildDate()} {upgradeTag()}
    </p>

    <p class="text-center text-muted">
      Copyright &copy; 2014-{new Date().getFullYear()} the Syncthing Authors.
    </p>

    <p class="text-center text-muted">
      Syncthing is Free and Open Source Software licensed as MPL v2.0.
    </p>

    <!-- Tabs -->
    <ul class="nav nav-tabs" style="margin-top: 20px;">
      <li class:active={activeTab === 'authors'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'authors'; }}>{$translations, t('Authors')}</a>
      </li>
      <li class:active={activeTab === 'software'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'software'; }}>{$translations, t('Included Software')}</a>
      </li>
      <li class:active={activeTab === 'paths'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'paths'; }}>{$translations, t('Paths')}</a>
      </li>
    </ul>

    <div class="tab-content">
      {#if activeTab === 'authors'}
        <h4 class="text-center">{$translations, t('The Syncthing Authors')}</h4>
        <div class="row">
          <div class="col-md-12" id="contributor-list">Jakob Borg, Audrius Butkevicius, Simon Frei, Tomasz Wilczyński, Alexander Graf, Alexandre Viau, Anderson Mesquita, André Colomb, Antony Male, Ben Schulz, bt90, Caleb Callaway, Daniel Harte, Emil Lundberg, Eric P, Evgeny Kuznetsov, greatroar, Lars K.W. Gohlke, Lode Hoste, Marcus B Spencer, Michael Ploujnikov, Ross Smith II, Stefan Tatschner, Tommy van der Vorst, Wulf Weich, Adam Piggott, Adel Qalieh, Aleksey Vasenev, Alessandro G., Alex Ionescu, Alex Lindeman, Alex Xu, Alexander Seiler, Alexandre Alves, Aman Gupta, Andreas Sommer, andresvia, Andrew Rabert, Andrey D, andyleap, Anjan Momi, Anthony Goeckner, Antoine Lamielle, Anur, Aranjedeath, ardevd, Arkadiusz Tymiński, Aroun, Arthur Axel fREW Schmidt, Artur Zubilewicz, Ashish Bhate, Aurélien Rainone, BAHADIR YILMAZ, Bart De Vries, Beat Reichenbach, Ben Norcombe, Ben Shepherd, Ben Sidhom, Benedikt Heine, Benno Fünfstück, Benny Ng, boomsquared, Boqin Qin, Boris Rybalkin, Brendan Long, Catfriend1, Cathryne Linenweaver, Cedric Staniewski, Chih-Hsuan Yen, Choongkyu, Chris Howie, Chris Joel, Christian Kujau, Christian Prescott, chucic, cjc7373, Colin Kennedy, Cromefire_, cui, Cyprien Devillez, d-volution, Dan, Daniel Barczyk, Daniel Bergmann, Daniel Martí, Daniel Padrta, Daniil Gentili, Darshil Chanpura, dashangcun, David Rimmer, DeflateAwning, Denis A., Dennis Wilson, derekriemer, DerRockWolf, desbma, Devon G. Redekopp, digital, Dimitri Papadopoulos Orfanos, Dmitry Saveliev, domain, Domenic Horner, Dominik Heidler, Elias Jarlebring, Elliot Huffman, Emil Hessman, Eng Zer Jun, entity0xfe, Epifeny, epifeny, Eric Lesiuta, Erik Meitner, Evan Spensley, Federico Castagnini, Felix, Felix Ableitner, Felix Lampe, Felix Unterpaintner, Francois-Xavier Gsell, Frank Isemann, Gahl Saraf, georgespatton, ghjklw, Gilli Sigurdsson, Gleb Sinyavskiy, Graham Miln, Greg, guangwu, gudvinr, Gusted, Han Boetes, HansK-p, Harrison Jones, Hazem Krimi, Heiko Zuerker, Hireworks, Hugo Locurcio, Iain Barnett, Ian Johnson, ignacy123, Iskander Sharipov, Jaakko Hannikainen, Jack Croft, Jacob, Jake Peterson, James O'Beirne, James Patterson, Jaroslav Lichtblau, Jaroslav Malec, Jaspitta, Jaya Chithra, Jaya Kumar, Jeffery To, jelle van der Waa, Jens Diemer, Jochen Voss, Johan Vromans, John Rinehart, Jonas Thelemann, Jonathan, Jose Manuel Delicado, JRNitre, jtagcat, Julian Lehrhuber, Jörg Thalheim, Jędrzej Kula, Kapil Sareen, Karol Różycki, Kebin Liu, Keith Harrison, Kelong Cong, Ken'ichi Kamada, Kevin Allen, Kevin Bushiri, Kevin White, Jr., klemens, Kurt Fitzner, kylosus, Lars Lehtonen, Laurent Etiemble, Leo Arias, Liu Siyuan, Lord Landon Agahnim, LSmithx2, Luiz Angelo Daros de Luca, Lukas Lihotzki, Luke Hamburg, luzpaz, Majed Abdulaziz, Marc Laporte, Marcel Meyer, Marcin Dziadus, Marcus Legendre, Mario Majila, Mark Pulford, Martchus, Mateusz Naściszewski, Mateusz Ż, mathias4833, Matic Potočnik, Matt Burke, Matt Robenolt, Matteo Ruina, Maurizio Tomasi, Max, Max Schulze, MaximAL, Maximilian, Maxwell G, Michael Jephcote, Michael Rienstra, Michael Wang 汪東陽, MichaIng, Migelo, Mike Boone, MikeLund, MikolajTwarog, Mingxuan Lin, mv1005, Nate Morrison, nf, Nicholas Rishel, Nick Busey, Nico Stapelbroek, Nicolas Braud-Santoni, Nicolas Perraut, Niels Peter Roest, Nils Jakobi, NinoM4ster, Nitroretro, NoLooseEnds, Oliver Freyermuth, orangekame3, otbutz, overkill, Oyebanji Jacob Mayowa, Pablo, Pascal Jungblut, Paul Brit, Paul Donald, Pawel Palenica, perewa, Peter Badida, Peter Dave Hello, Peter Hoeg, Peter Marquardt, Phani Rithvij, Phil Davis, Philippe Schommers, Phill Luby, Piotr Bejda, polyfloyd, Prathik P Kulkarni, pullmerge, Quentin Hibon, Rahmi Pruitt, RealCharlesChia, red_led, Robert Carosi, Roberto Santalla, Robin Schoonover, Roman Zaynetdinov, rubenbe, Ruslan Yevdokymov, Ryan Qian, Ryan Sullivan, Sacheendra Talluri, Scott Klupfel, sec65, Sergey Mishin, Sertonix, Severin von Wnuck-Lipinski, Shaarad Dalvi, Shivam Kumar, Simon Mwepu, Simon Pickup, Sly_tom_cat, Sonu Kumar Saw, Stefan Kuntz, Steven Eckhoff, Suhas Gundimeda, Sven Bachmann, Sébastien WENSKE, Tao, Taylor Khan, Terrance, TheCreeper, Thomas, Thomas Hipp, Tim Abell, Tim Howes, Tobias Frölich, Tobias Klauser, Tobias Nygren, Tobias Tom, Tom Jakubowski, Tully Robinson, Tyler Brazier, Tyler Kropp, Umer-Azaz, Unrud, Val Markovic, vapatel2, Veeti Paananen, Victor Buinsky, Vik, Vil Brekin, villekalliomaki, Vladimir Rusinov, vvaswani, wangguoliang, WangXi, Will Rouesnel, William A. Kennington III, wouter bolsterlee, xarx00, Xavier O., xjtdy888, Yannic A., yparitcher, 佛跳墙, 落心</div>
        </div>
      {/if}
      {#if activeTab === 'software'}
        <p>{$translations, t('Syncthing includes the following software or portions thereof:')}</p>
        <ul class="list-unstyled two-columns" id="copyright-notices">
          <li><a href="https://getbootstrap.com/">Bootstrap</a>, Copyright &copy; 2011-2016 Twitter, Inc.</li>
          <li><a href="https://svelte.dev/">Svelte</a>, Copyright &copy; 2016-2024 Rich Harris and contributors.</li>
          <li><a href="https://fontawesome.com/">Font Awesome</a>, Copyright &copy; 2024 Fonticons, Inc.</li>
          <li><a href="https://forkaweso.me/Fork-Awesome/">Fork Awesome</a>, Copyright &copy; 2018 Dave Gandy &amp; Fork Awesome.</li>
          <li><a href="https://golang.org/">The Go Programming Language</a>, Copyright &copy; 2009 The Go Authors.</li>
          <li><a href="https://prometheus.io/">Prometheus</a>, Copyright &copy; 2012-2015 The Prometheus Authors.</li>
          <li><a href="https://github.com/syncthing/syncthing/blob/main/go.sum" target="_blank">{t('Full list of Go dependencies on GitHub')}</a></li>
        </ul>
      {/if}
      {#if activeTab === 'paths'}
        <table class="table table-striped table-auto">
          <caption><label>{$translations, t('Internally used paths:')}</label></caption>
          <tbody>
            {#if paths['baseDir-userHome']}
              <tr><th>{t('User Home')}</th><td><code class="word-break-all">{paths['baseDir-userHome']}</code></td></tr>
            {/if}
            {#if paths['baseDir-config']}
              <tr><th><strong>{t('Configuration Directory')}</strong></th><td><code class="word-break-all"><strong>{paths['baseDir-config']}</strong></code></td></tr>
            {/if}
            {#if paths['config']}
              <tr><th>{t('Configuration File')}</th><td><code class="word-break-all">{paths['config']}</code></td></tr>
            {/if}
            {#if paths['certFile']}
              <tr><th>{t('Device Certificate')}</th><td><code class="word-break-all">{paths['certFile']}</code><br /><code class="word-break-all">{paths['keyFile'] || ''}</code></td></tr>
            {/if}
            {#if paths['httpsCertFile']}
              <tr><th>{t('GUI / API HTTPS Certificate')}</th><td><code class="word-break-all">{paths['httpsCertFile']}</code><br /><code class="word-break-all">{paths['httpsKeyFile'] || ''}</code></td></tr>
            {/if}
            {#if paths['database']}
              <tr><th>{t('Database Location')}</th><td><code class="word-break-all">{paths['database']}</code></td></tr>
            {/if}
            {#if paths['logFile']}
              <tr><th>{t('Log File')}</th><td><code class="word-break-all">{paths['logFile']}</code></td></tr>
            {/if}
            {#if paths['guiAssets']}
              <tr><th>{t('GUI Override Directory')}</th><td><code class="word-break-all">{paths['guiAssets']}</code></td></tr>
            {/if}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default" onclick={onclose}>{$translations, t('Close')}</button>
  </div>
</Modal>
