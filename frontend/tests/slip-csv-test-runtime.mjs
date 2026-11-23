import vm from 'node:vm';
// Focused monetary regression tests supply displayed records; selection and DOM
// synchronization are tested with the real renderer in slip-csv-selection.test.
export function installCsvRuntime(source, context, records, selected = () => new Set()) {
  const helpersStart = source.indexOf('function slipCsvNumber(');
  vm.runInContext(source.slice(helpersStart, source.indexOf('/** One row per displayed shipment', helpersStart)), context);
  const idStart = source.indexOf('function _slipRecordId(');
  vm.runInContext(source.slice(idStart, source.indexOf('function _slipResetAllSelections(', idStart)), context);
  const start = source.indexOf('function captureSlipCsvRecords(');
  const end = source.indexOf('/** Snapshot the displayed purchase slips', start);
  vm.runInContext(source.slice(start, end), context);
  context.getSlipCsvRecords = () => records().filter(row => !selected().size || selected().has(row.id));
  context.document.body ||= {appendChild(){}};
  context.document.getElementById ||= () => null;
  context.document.querySelectorAll ||= () => [];
  const create = context.document.createElement;
  context.document.createElement = (...args) => Object.assign({remove(){}}, create(...args));
  context.URL.revokeObjectURL ||= () => {};
  context.setTimeout ||= fn => fn();
  context.getLocalDateISO ||= () => '2026-10-11';
}
