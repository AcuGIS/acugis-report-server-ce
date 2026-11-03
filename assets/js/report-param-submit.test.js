'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const params = require('./report-param-submit.js');

test('blank string parameter is present as empty string', () => {
  assert.equal(params.scalarQueryValue({ name: 'property_id', type: 'string' }, ''), '');
  assert.equal(params.scalarQueryValue({ name: 'property_id', type: 'text' }, ''), '');
  assert.equal(params.scalarQueryValue({ name: 'property_id' }, ''), '');
  const url = params.appendScalarToUrl('http://reports/execute?_repName=property-pro', 'property_id', '');
  assert.equal(url, 'http://reports/execute?_repName=property-pro&property_id=');
});

test('blank numeric date and other typed parameters stay omitted', () => {
  for (const type of ['integer', 'number', 'double', 'float', 'long', 'decimal', 'date', 'datetime', 'boolean', 'select', 'hidden']) {
    assert.equal(params.scalarQueryValue({ name: 'year', type: type }, ''), null, type);
  }
  assert.equal(params.scalarQueryValue({ type: 'string' }, '', { preserveBlank: false }), null);
  assert.equal(params.scalarQueryValue({ type: 'integer' }, '', { explicitEmpty: true, fallbackDefault: '2026' }), null);
  assert.equal(
    params.appendScalarToUrl('http://reports/execute?_repName=property-pro', 'year', null),
    'http://reports/execute?_repName=property-pro'
  );
});

test('nonblank string is submitted unchanged', () => {
  assert.equal(
    params.scalarQueryValue({ name: 'property_id', type: 'string' }, 'hilltop-manor'),
    'hilltop-manor'
  );
  const url = params.appendScalarToUrl('http://reports/execute?_repFormat=html', 'property_id', 'hilltop-manor');
  assert.equal(url, 'http://reports/execute?_repFormat=html&property_id=hilltop-manor');
});

test('configured default still fills an untouched blank control', () => {
  assert.equal(
    params.scalarQueryValue({ type: 'string' }, '', { fallbackDefault: 'smith-residential' }),
    'smith-residential'
  );
  assert.equal(
    params.scalarQueryValue({ type: 'string' }, '', { explicitEmpty: true, fallbackDefault: 'smith-residential' }),
    ''
  );
  assert.equal(
    params.scalarQueryValue({ type: 'number' }, '', { fallbackDefault: '2026' }),
    '2026'
  );
});

test('hidden and auto-sourced blanks stay omitted', () => {
  assert.equal(params.scalarQueryValue({ type: 'string' }, '', { hidden: true }), null);
  assert.equal(params.scalarQueryValue({ type: 'string' }, '', { autoSourced: true }), null);
  assert.equal(params.scalarQueryValue({ type: 'string' }, 'kept', { hidden: true }), 'kept');
});
