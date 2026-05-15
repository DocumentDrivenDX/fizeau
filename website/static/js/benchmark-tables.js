(function () {
  'use strict';

  var TABLE_SELECTOR = '.br-body table, .bench-comparison__table, .rp-table';
  var enhanced = 'benchTableEnhanced';
  var lightbox = null;

  function ready(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn, { once: true });
    } else {
      fn();
    }
  }

  function textOf(el) {
    return (el && el.textContent ? el.textContent : '').replace(/\s+/g, ' ').trim();
  }

  function nearestHeading(table) {
    var node = table.closest('.bench-comparison__block, .br-body, main');
    if (!node) return 'Benchmark table';

    if (node.classList.contains('bench-comparison__block')) {
      var title = node.querySelector('.bench-comparison__title');
      if (title) return textOf(title);
    }

    var cur = table;
    while (cur && cur !== node) {
      var prev = cur.previousElementSibling;
      while (prev) {
        if (/^H[1-6]$/.test(prev.tagName)) return textOf(prev);
        var nested = prev.querySelector && prev.querySelector('h1, h2, h3, h4, h5, h6');
        if (nested) return textOf(nested);
        prev = prev.previousElementSibling;
      }
      cur = cur.parentElement;
    }

    var caption = table.querySelector('caption');
    if (caption) return textOf(caption);
    return 'Benchmark table';
  }

  function tableStats(table) {
    var rows = table.tBodies.length ? table.tBodies[0].rows.length : table.querySelectorAll('tr').length;
    var headCols = table.tHead && table.tHead.rows.length ? table.tHead.rows[0].cells.length : 0;
    var bodyCols = 0;
    Array.prototype.forEach.call(table.rows, function (row) {
      bodyCols = Math.max(bodyCols, row.cells.length);
    });
    var cols = Math.max(headCols, bodyCols);
    return { rows: rows, cols: cols, label: rows + ' rows x ' + cols + ' columns' };
  }

  function makeButton(label, className) {
    var button = document.createElement('button');
    button.type = 'button';
    button.className = 'bench-table-btn' + (className ? ' ' + className : '');
    button.textContent = label;
    return button;
  }

  function designCssLoaded() {
    return getComputedStyle(document.documentElement).getPropertyValue('--surface-panel').trim() !== '';
  }

  function installFilePreviewFallback() {
    if (document.getElementById('bench-table-file-preview-fallback')) return;
    var style = document.createElement('style');
    style.id = 'bench-table-file-preview-fallback';
    style.textContent = [
      '*,*::before,*::after{box-sizing:border-box}',
      'body{margin:0;font-family:system-ui,sans-serif}',
      '.hextra-sidebar-container{display:none!important}',
      '.hextra-max-page-width{display:block!important;max-width:none!important}',
      'article,main#content{display:block!important;width:auto!important;max-width:none!important}',
      'main#content{padding:24px!important}',
      'p,li,.narrative,.meta{max-width:100%;overflow-wrap:anywhere;word-break:break-word}',
      'img,svg{max-width:100%;height:auto}',
      '.rp-table-wrap,.br-body table,.bench-comparison__table{overflow:auto;max-width:100%}',
      '.rp-th{cursor:pointer}',
      '@media(max-width:640px){main#content{padding:16px!important}}'
    ].join('');
    document.head.appendChild(style);
  }

  function enhanceAllTables() {
    Array.prototype.forEach.call(document.querySelectorAll(TABLE_SELECTOR), enhanceTable);
  }

  function enhanceWhenStylesAreReady(attempt) {
    if (designCssLoaded()) {
      enhanceAllTables();
      return;
    }
    if (window.location.protocol === 'file:') {
      installFilePreviewFallback();
      return;
    }
    if (attempt >= 20) {
      installFilePreviewFallback();
      return;
    }
    window.setTimeout(function () {
      enhanceWhenStylesAreReady(attempt + 1);
    }, 50);
  }

  function enhanceTable(table) {
    if (!table || table.dataset[enhanced] === 'true') return;
    if (table.closest('.bench-table-shell, .bench-table-lightbox')) return;

    var parent = table.parentElement;
    if (!parent) return;

    var title = nearestHeading(table);
    var stats = tableStats(table);
    var subject = parent.classList.contains('rp-table-wrap') ? parent : table;
    var subjectParent = subject.parentElement;
    if (!subjectParent) return;

    var shell = document.createElement('div');
    shell.className = 'bench-table-shell';

    var toolbar = document.createElement('div');
    toolbar.className = 'bench-table-toolbar';

    var titleEl = document.createElement('div');
    titleEl.className = 'bench-table-title';
    titleEl.textContent = title;

    var statsEl = document.createElement('div');
    statsEl.className = 'bench-table-stats';
    statsEl.textContent = stats.label;

    var actions = document.createElement('div');
    actions.className = 'bench-table-actions';

    var expand = makeButton('Expand');
    expand.setAttribute('aria-label', 'Expand ' + title);
    expand.addEventListener('click', function () {
      openLightbox(table, title);
    });

    actions.appendChild(expand);
    toolbar.appendChild(titleEl);
    toolbar.appendChild(statsEl);
    toolbar.appendChild(actions);

    subjectParent.insertBefore(shell, subject);
    shell.appendChild(toolbar);

    if (subject === table) {
      var viewport = document.createElement('div');
      viewport.className = 'bench-table-viewport';
      viewport.appendChild(table);
      shell.appendChild(viewport);
    } else {
      subject.classList.add('bench-table-viewport');
      shell.appendChild(subject);
    }

    table.dataset[enhanced] = 'true';
  }

  function stripIds(root) {
    if (root.removeAttribute) root.removeAttribute('id');
    Array.prototype.forEach.call(root.querySelectorAll('[id]'), function (el) {
      el.removeAttribute('id');
    });
  }

  function ensureLightbox() {
    if (lightbox) return lightbox;

    lightbox = document.createElement('div');
    lightbox.className = 'bench-table-lightbox';
    lightbox.setAttribute('role', 'dialog');
    lightbox.setAttribute('aria-modal', 'true');
    lightbox.hidden = true;

    var bar = document.createElement('div');
    bar.className = 'bench-table-lightbox__bar';

    var title = document.createElement('div');
    title.className = 'bench-table-lightbox__title';

    var stats = document.createElement('div');
    stats.className = 'bench-table-lightbox__stats';

    var close = makeButton('Close', 'bench-table-lightbox__close');
    close.addEventListener('click', closeLightbox);

    var viewport = document.createElement('div');
    viewport.className = 'bench-table-lightbox__viewport';

    bar.appendChild(title);
    bar.appendChild(stats);
    bar.appendChild(close);
    lightbox.appendChild(bar);
    lightbox.appendChild(viewport);
    document.body.appendChild(lightbox);

    lightbox.addEventListener('click', function (event) {
      if (event.target === lightbox) closeLightbox();
    });
    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && !lightbox.hidden) closeLightbox();
    });

    return lightbox;
  }

  function openLightbox(table, title) {
    var box = ensureLightbox();
    var clone = table.cloneNode(true);
    stripIds(clone);
    clone.removeAttribute('style');
    clone.dataset[enhanced] = 'true';

    var stats = tableStats(table);
    box.querySelector('.bench-table-lightbox__title').textContent = title;
    box.querySelector('.bench-table-lightbox__stats').textContent = stats.label;

    var viewport = box.querySelector('.bench-table-lightbox__viewport');
    viewport.innerHTML = '';
    viewport.appendChild(clone);

    box.hidden = false;
    document.body.classList.add('bench-table-lock');
    box.querySelector('.bench-table-lightbox__close').focus();
  }

  function closeLightbox() {
    if (!lightbox) return;
    lightbox.hidden = true;
    document.body.classList.remove('bench-table-lock');
    var viewport = lightbox.querySelector('.bench-table-lightbox__viewport');
    if (viewport) viewport.innerHTML = '';
  }

  ready(function () {
    enhanceWhenStylesAreReady(0);
  });
})();
