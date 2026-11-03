<div id="reportParamModal" class="fixed inset-0 bg-black bg-opacity-50 hidden z-[9999]">
  <div class="flex items-center justify-center h-full">
    <div class="bg-white rounded-lg w-full max-w-lg p-6">
      <h2 class="text-lg font-semibold mb-4">Run Report</h2>
      <form id="reportParamForm" class="space-y-4"></form>

      <div class="flex justify-end gap-2 mt-4">
        <button type="button" id="cancelReport" class="px-4 py-2 bg-gray-300 rounded">Cancel</button>
        <button type="button" id="runReport" class="px-4 py-2 bg-blue-600 text-white rounded">Run</button>
      </div>
    </div>
  </div>
</div>

<script>
let currentReport = null;

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function runReport(card) {
  if (!card) {
    return;
  }
  const repName = card.dataset.report || '';
  const dataSource = card.dataset.datasource || '';
  const repFormat = card.dataset.format || 'html';
  let url = `/report?_repName=${encodeURIComponent(repName)}&_dataSource=${encodeURIComponent(dataSource)}&_repFormat=${encodeURIComponent(repFormat)}`;
  window.open(url, '_blank');
}

document.querySelectorAll('.report-card').forEach(card => {
  card.addEventListener('click', function (e) {
    const href = this.getAttribute('href') || '';
    if (href && href !== '#') {
      // Report cards now open dedicated viewer page.
      return;
    }
    e.preventDefault();

    const params = JSON.parse(this.dataset.params || '[]');

    if (!params.length) {
      runReport(this);
      return;
    }

    currentReport = this;
    buildForm(params);
    document.getElementById('reportParamModal').classList.remove('hidden');
  });
});

function buildForm(params) {
  const form = document.getElementById('reportParamForm');
  form.innerHTML = '';

  params.forEach(p => {
    let field = '';

    if (p.type === 'select') {
      field = `<select name="${p.name}" ${p.multiple ? 'multiple' : ''} class="w-full border p-2">`;

      (p.options || []).forEach(opt => {
        if (opt && typeof opt === 'object') {
          const optionLabel = opt.label ?? opt.value ?? '';
          const optionValue = opt.value ?? opt.label ?? '';
          field += `<option value="${escapeHtml(optionValue)}">${escapeHtml(optionLabel)}</option>`;
        } else {
          field += `<option value="${escapeHtml(opt)}">${escapeHtml(opt)}</option>`;
        }
      });

      field += `</select>`;
    } else if (p.type === 'date') {
      field = `<input type="date" name="${p.name}" class="w-full border p-2">`;
    } else if (p.type === 'number') {
      field = `<input type="number" name="${p.name}" class="w-full border p-2">`;
    } else {
      field = `<input type="text" name="${p.name}" class="w-full border p-2">`;
    }

    form.innerHTML += `
      <div>
        <label class="block text-sm font-medium mb-1">${p.label || p.name}</label>
        ${field}
      </div>
    `;
  });
}

document.getElementById('runReport').addEventListener('click', function () {
  if (!currentReport) {
    return;
  }
  let url = `/report?_repName=${currentReport.dataset.report}&_dataSource=${currentReport.dataset.datasource}&_repFormat=${currentReport.dataset.format}`;
  const form = document.getElementById('reportParamForm');

  form.querySelectorAll('[name]').forEach(el => {
    const name = el.name;

    if (el.tagName === 'SELECT' && el.multiple) {
      Array.from(el.selectedOptions).forEach(opt => {
        url += `&${name}=${encodeURIComponent(opt.value)}`;
      });
    } else {
      if (el.value) {
        url += `&${name}=${encodeURIComponent(el.value)}`;
      }
    }
  });

  console.log(url);
  window.open(url, '_blank');
  document.getElementById('reportParamModal').classList.add('hidden');
});

document.getElementById('cancelReport').addEventListener('click', function () {
  document.getElementById('reportParamModal').classList.add('hidden');
});
</script>
