import { useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type {
  ModbusRequest,
  ModbusResult,
  ModbusScanStatus,
  ModbusWritePreview,
  ModbusCommand,
} from '../../api/modbus';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  endpointId: string;
  transport: 'tcp' | 'rtu_tcp' | 'rtu';
  sessionId: string;
  canControl: boolean;
}

/** Deliberate bounded Modbus tools on an already approved live lease. */
export function ModbusToolsPanel({
  client,
  credential,
  endpointId,
  transport,
  sessionId,
  canControl,
}: Props) {
  const t = useTranslate();
  const [functionCode, setFunctionCode] = useState(3);
  const [unitFrom, setUnitFrom] = useState(1);
  const [unitTo, setUnitTo] = useState(1);
  const [addressFrom, setAddressFrom] = useState(0);
  const [addressTo, setAddressTo] = useState(0);
  const [rate, setRate] = useState(2);
  const [deadline, setDeadline] = useState(10);
  const [scan, setScan] = useState<ModbusScanStatus>();
  const [diagnosticKind, setDiagnosticKind] = useState<
    'device_information' | 'comm_event_counter' | 'report_server_id'
  >('device_information');
  const [diagnosticUnit, setDiagnosticUnit] = useState(1);
  const [diagnostic, setDiagnostic] = useState<ModbusResult>();
  const [writeFunction, setWriteFunction] = useState<22 | 23>(22);
  const [writeUnit, setWriteUnit] = useState(1);
  const [writeAddress, setWriteAddress] = useState(0);
  const [andMask, setAndMask] = useState(65535);
  const [orMask, setOrMask] = useState(0);
  const [readAddress, setReadAddress] = useState(0);
  const [readCount, setReadCount] = useState(1);
  const [writeValues, setWriteValues] = useState('0');
  const [preview, setPreview] = useState<ModbusWritePreview>();
  const [previewKey, setPreviewKey] = useState('');
  const [command, setCommand] = useState<ModbusCommand>();
  const [attempted, setAttempted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (scan?.state !== 'running' || !client.getModbusScan) return;
    let active = true;
    const timer = window.setInterval(async () => {
      try {
        const next = await client.getModbusScan!(credential, endpointId, sessionId);
        if (active) setScan(next);
      } catch (cause) {
        if (active)
          setError(
            cause instanceof BenchCoreError
              ? cause.code || cause.message
              : t('modbus.unavailable'),
          );
      }
    }, 500);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [scan?.state, client, credential, endpointId, sessionId, t]);

  const report = (cause: unknown) =>
    setError(
      cause instanceof BenchCoreError
        ? cause.code || cause.message
        : t('modbus.unavailable'),
    );
  const startScan = async () => {
    const total = (unitTo - unitFrom + 1) * (addressTo - addressFrom + 1);
    if (
      !canControl ||
      !client.startModbusScan ||
      ![1, 2, 3, 4].includes(functionCode) ||
      unitFrom < 0 ||
      unitTo > 255 ||
      unitTo < unitFrom ||
      addressFrom < 0 ||
      addressTo > 65535 ||
      addressTo < addressFrom ||
      total > 512 ||
      rate < 1 ||
      rate > 10 ||
      deadline < 1 ||
      deadline > 60
    ) {
      setError(t('modbus.scanInvalid'));
      return;
    }
    setBusy(true);
    setError('');
    try {
      setScan(
        await client.startModbusScan(credential, endpointId, {
          session_id: sessionId,
          function: functionCode,
          unit_from: unitFrom,
          unit_to: unitTo,
          address_from: addressFrom,
          address_to: addressTo,
          rate_per_second: rate,
          deadline_seconds: deadline,
        }),
      );
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  };
  const cancelScan = async () => {
    if (!client.cancelModbusScan) return;
    try {
      setScan(await client.cancelModbusScan(credential, endpointId, sessionId));
    } catch (cause) {
      report(cause);
    }
  };
  const exportScan = async (format: 'csv' | 'jsonl') => {
    if (!client.downloadModbusScan) return;
    try {
      const blob = await client.downloadModbusScan(
        credential,
        endpointId,
        sessionId,
        format,
      );
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `modbus-scan-${scan?.scan_id ?? 'current'}.${format}`;
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (cause) {
      report(cause);
    }
  };
  const readDiagnostic = async () => {
    if (
      !client.readModbusDiagnostic ||
      !canControl ||
      (transport === 'tcp' && diagnosticKind !== 'device_information')
    )
      return;
    setBusy(true);
    try {
      setDiagnostic(
        await client.readModbusDiagnostic(
          credential,
          endpointId,
          sessionId,
          diagnosticKind,
          diagnosticUnit,
        ),
      );
      setError('');
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  };
  const makeWrite = (): ModbusRequest | undefined => {
    if (writeFunction === 22) {
      if (
        ![writeUnit, writeAddress, andMask, orMask].every(Number.isInteger) ||
        writeUnit < 0 ||
        writeUnit > 255 ||
        writeAddress < 0 ||
        writeAddress > 65535 ||
        andMask < 0 ||
        andMask > 65535 ||
        orMask < 0 ||
        orMask > 65535
      )
        return;
      return {
        function: 22,
        unit: writeUnit,
        address: writeAddress,
        count: 1,
        values: [],
        and_mask: andMask,
        or_mask: orMask,
      };
    }
    const values = writeValues.split(',').map((value) => Number(value.trim()));
    if (
      writeValues.split(',').some((value) => value.trim() === '') ||
      !values.length ||
      values.length > 121 ||
      values.some((value) => !Number.isInteger(value) || value < 0 || value > 65535) ||
      ![writeUnit, writeAddress, readAddress, readCount].every(Number.isInteger) ||
      writeUnit < 0 ||
      writeUnit > 255 ||
      writeAddress < 0 ||
      writeAddress + values.length > 65536 ||
      readAddress < 0 ||
      readCount < 1 ||
      readCount > 125 ||
      readAddress + readCount > 65536
    )
      return;
    return {
      function: 23,
      unit: writeUnit,
      address: writeAddress,
      count: values.length,
      values,
      read_address: readAddress,
      read_count: readCount,
    };
  };
  const previewWrite = async () => {
    const request = makeWrite();
    if (!request || !client.previewModbusWrite || !canControl) {
      setError(t('modbus.writeInvalid'));
      return;
    }
    const key = crypto.randomUUID();
    setBusy(true);
    setError('');
    setPreview(undefined);
    setCommand(undefined);
    setAttempted(false);
    try {
      setPreview(
        await client.previewModbusWrite(
          credential,
          endpointId,
          sessionId,
          key,
          request,
        ),
      );
      setPreviewKey(key);
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  };
  const submitWrite = async () => {
    if (!preview || !client.writeModbus || attempted) return;
    setAttempted(true);
    setBusy(true);
    try {
      setCommand(
        await client.writeModbus(
          credential,
          endpointId,
          sessionId,
          previewKey,
          preview.request,
        ),
      );
      setError('');
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  };
  const numberField = (
    label: string,
    value: number,
    change: (value: number) => void,
    min: number,
    max: number,
  ) => (
    <label>
      {label}
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => change(Number(event.target.value))}
      />
    </label>
  );
  const changeWrite = (change: () => void) => {
    change();
    setPreview(undefined);
    setCommand(undefined);
    setAttempted(false);
  };

  return (
    <section className="instrument-card modbus-tools" aria-label={t('modbus.tools')}>
      <h3>{t('modbus.tools')}</h3>
      {error && <MessageBanner tone="error">{error}</MessageBanner>}
      <fieldset disabled={!canControl || busy}>
        <legend>{t('modbus.scan')}</legend>
        <label>
          {t('modbus.function')}
          <select
            value={functionCode}
            onChange={(event) => setFunctionCode(Number(event.target.value))}
          >
            {[1, 2, 3, 4].map((fn) => (
              <option key={fn} value={fn}>
                FC{fn.toString().padStart(2, '0')}
              </option>
            ))}
          </select>
        </label>
        {numberField(t('modbus.unitFrom'), unitFrom, setUnitFrom, 0, 255)}
        {numberField(t('modbus.unitTo'), unitTo, setUnitTo, 0, 255)}
        {numberField(t('modbus.addressFrom'), addressFrom, setAddressFrom, 0, 65535)}
        {numberField(t('modbus.addressTo'), addressTo, setAddressTo, 0, 65535)}
        {numberField(t('modbus.rate'), rate, setRate, 1, 10)}
        {numberField(t('modbus.deadline'), deadline, setDeadline, 1, 60)}
        <div className="modbus-actions">
          <button
            type="button"
            disabled={scan?.state === 'running'}
            onClick={() => void startScan()}
          >
            {t('modbus.startScan')}
          </button>
          <button
            type="button"
            disabled={scan?.state !== 'running'}
            onClick={() => void cancelScan()}
          >
            {t('modbus.cancelScan')}
          </button>
        </div>
      </fieldset>
      {scan && (
        <div>
          <p role="status">
            {scan.state} · {scan.completed}/{scan.total}
          </p>
          <div className="modbus-actions">
            <button type="button" onClick={() => void exportScan('csv')}>
              {t('modbus.exportCsv')}
            </button>
            <button type="button" onClick={() => void exportScan('jsonl')}>
              {t('modbus.exportJsonl')}
            </button>
          </div>
          <div className="modbus-scan-results">
            <table>
              <thead>
                <tr>
                  <th>{t('modbus.function')}</th>
                  <th>{t('modbus.unit')}</th>
                  <th>{t('modbus.address')}</th>
                  <th>{t('modbus.outcome')}</th>
                  <th>{t('modbus.raw')}</th>
                  <th>{t('modbus.hex')}</th>
                </tr>
              </thead>
              <tbody>
                {scan.results.map((probe) => (
                  <tr key={`${probe.unit}:${probe.address}`}>
                    <td>FC{probe.function}</td>
                    <td>{probe.unit}</td>
                    <td>{probe.address}</td>
                    <td>{probe.outcome}</td>
                    <td>{probe.values.join(', ') || '—'}</td>
                    <td>
                      {probe.frames?.length ? (
                        <details>
                          <summary>{t('modbus.hex')}</summary>
                          {probe.frames.map((frame, index) => (
                            <div key={index}>
                              {frame.direction.toUpperCase()}:{' '}
                              <code>{frame.data_hex}</code>
                            </div>
                          ))}
                        </details>
                      ) : (
                        '—'
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
      <fieldset disabled={!canControl || busy}>
        <legend>{t('modbus.diagnostic')}</legend>
        <label>
          {t('modbus.diagnosticKind')}
          <select
            value={diagnosticKind}
            onChange={(event) =>
              setDiagnosticKind(event.target.value as typeof diagnosticKind)
            }
          >
            <option value="device_information">FC43/14</option>
            <option value="comm_event_counter" disabled={transport === 'tcp'}>
              FC11
            </option>
            <option value="report_server_id" disabled={transport === 'tcp'}>
              FC17
            </option>
          </select>
        </label>
        {numberField(t('modbus.unit'), diagnosticUnit, setDiagnosticUnit, 1, 247)}
        <button
          type="button"
          disabled={transport === 'tcp' && diagnosticKind !== 'device_information'}
          onClick={() => void readDiagnostic()}
        >
          {t('modbus.readDiagnostic')}
        </button>
        {diagnostic && (
          <pre role="status">
            {JSON.stringify(
              diagnostic.diagnostic ?? {
                outcome: diagnostic.outcome,
                exception_code: diagnostic.exception_code,
              },
              null,
              2,
            )}
          </pre>
        )}
      </fieldset>
      <fieldset disabled={!canControl || busy}>
        <legend>{t('modbus.advancedWrite')}</legend>
        <label>
          {t('modbus.function')}
          <select
            value={writeFunction}
            onChange={(event) =>
              changeWrite(() => setWriteFunction(Number(event.target.value) as 22 | 23))
            }
          >
            <option value={22}>FC22</option>
            <option value={23}>FC23</option>
          </select>
        </label>
        {numberField(
          t('modbus.unit'),
          writeUnit,
          (value) => changeWrite(() => setWriteUnit(value)),
          0,
          255,
        )}
        {numberField(
          t('modbus.address'),
          writeAddress,
          (value) => changeWrite(() => setWriteAddress(value)),
          0,
          65535,
        )}
        {writeFunction === 22 ? (
          <>
            {numberField(
              t('modbus.andMask'),
              andMask,
              (value) => changeWrite(() => setAndMask(value)),
              0,
              65535,
            )}
            {numberField(
              t('modbus.orMask'),
              orMask,
              (value) => changeWrite(() => setOrMask(value)),
              0,
              65535,
            )}
          </>
        ) : (
          <>
            {numberField(
              t('modbus.readAddress'),
              readAddress,
              (value) => changeWrite(() => setReadAddress(value)),
              0,
              65535,
            )}
            {numberField(
              t('modbus.readCount'),
              readCount,
              (value) => changeWrite(() => setReadCount(value)),
              1,
              125,
            )}
            <label>
              {t('modbus.writeValues')}
              <input
                type="text"
                value={writeValues}
                onChange={(event) =>
                  changeWrite(() => setWriteValues(event.target.value))
                }
              />
            </label>
          </>
        )}
        <div className="modbus-actions">
          <button type="button" onClick={() => void previewWrite()}>
            {t('modbus.previewWrite')}
          </button>
          <button
            type="button"
            disabled={!preview || attempted}
            onClick={() => void submitWrite()}
          >
            {t('modbus.submitWrite')}
          </button>
        </div>
        {preview && (
          <p>
            PDU: <code>{preview.pdu_hex}</code> · {t('modbus.digest')}:{' '}
            <code>{preview.digest}</code>
          </p>
        )}
        {command && (
          <p role="status">
            {t('modbus.writeOutcome')}: {command.outcome}
          </p>
        )}
      </fieldset>
    </section>
  );
}
