import test from 'node:test';
import assert from 'node:assert/strict';
import { ContainerReadiness } from '../src/readiness.ts';

test('warm application requests perform readiness work only once', async () => {
  const readiness = new ContainerReadiness();
  let probes = 0;
  const initialize = async () => { probes++; };
  await readiness.ensure(true, initialize);
  for (let request = 0; request < 50; request++) await readiness.ensure(true, initialize);
  assert.equal(probes, 1);
});

test('concurrent cold requests share one startup and readiness check', async () => {
  const readiness = new ContainerReadiness();
  let probes = 0;
  let release;
  const started = new Promise(resolve => { release = resolve; });
  const initialize = async () => { probes++; await started; };
  const requests = Array.from({ length: 20 }, () => readiness.ensure(false, initialize));
  await Promise.resolve();
  assert.equal(probes, 1);
  release();
  await Promise.all(requests);
});

test('startup failure is not cached and a later request can recover', async () => {
  const readiness = new ContainerReadiness();
  let probes = 0;
  await assert.rejects(readiness.ensure(false, async () => { probes++; throw new Error('not ready'); }), /not ready/);
  await readiness.ensure(false, async () => { probes++; });
  await readiness.ensure(true, async () => { probes++; });
  assert.equal(probes, 2);
});

test('observed process exit invalidates readiness even before running state updates', async () => {
  const readiness = new ContainerReadiness();
  let probes = 0;
  const initialize = async () => { probes++; };
  await readiness.ensure(true, initialize);
  readiness.invalidate();
  await readiness.ensure(true, initialize);
  assert.equal(probes, 2);
});

test('stopped container is probed again even without an exit notification', async () => {
  const readiness = new ContainerReadiness();
  let probes = 0;
  const initialize = async () => { probes++; };
  await readiness.ensure(true, initialize);
  await readiness.ensure(false, initialize);
  assert.equal(probes, 2);
});
