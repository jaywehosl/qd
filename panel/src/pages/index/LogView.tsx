interface ParsedLog {
  date: string;
  time: string;
  stamp: string;
  levelText: string;
  levelClass: string;
  service: string;
  body: string;
}

const LEVELS = ['DEBUG', 'INFO', 'NOTICE', 'WARNING', 'ERROR'];
const LEVEL_CLASSES = ['level-debug', 'level-info', 'level-notice', 'level-warning', 'level-error'];

export function parseLogLine(line: string): ParsedLog {
  const [head, ...rest] = (line || '').split(' - ');
  const message = rest.join(' - ');
  const parts = head.split(' ');

  let date = '';
  let time = '';
  let levelText: string;
  if (parts.length >= 3) {
    [date, time, levelText] = parts;
  } else {
    levelText = head;
  }

  const li = LEVELS.indexOf(levelText);
  const levelClass = li >= 0 ? LEVEL_CLASSES[li] : 'level-unknown';

  let service = '';
  let body = message || '';
  if (body.startsWith('XRAY:')) {
    service = 'XRAY:';
    body = body.slice('XRAY:'.length).trimStart();
  } else if (body) {
    service = 'QD:';
  }

  const stamp = [date, time].filter(Boolean).join(' ');

  return { date, time, stamp, levelText, levelClass, service, body };
}

interface LogViewProps {
  lines: string[];
  isMobile: boolean;
  empty: string;
}

export default function LogView({ lines, isMobile, empty }: LogViewProps) {
  const parsed = lines.map(parseLogLine);

  if (parsed.length === 0) return <div className="log-empty">{empty}</div>;

  return (
    <div className={`log-container ${isMobile ? 'log-container-mobile' : ''}`}>
      {isMobile ? (
        parsed.map((log, idx) => (
          <div key={idx} className="log-card">
            <div className="log-card-head">
              {log.stamp && (
                <span className="log-time">
                  {log.time && <span>{log.time}</span>}
                  {log.time && log.date ? ' ' : ''}
                  {log.date && <span className="log-date">{log.date}</span>}
                </span>
              )}
              {log.levelText && (
                <span className={`log-level-badge ${log.levelClass}`}>{log.levelText}</span>
              )}
            </div>
            {(log.body || log.service) && (
              <div className="log-body">
                {log.service && <b>{log.service}</b>}
                {log.service && log.body ? ' ' : ''}
                {log.body && <span className="log-body-text">{log.body}</span>}
              </div>
            )}
          </div>
        ))
      ) : (
        parsed.map((log, idx) => (
          <div key={idx} className="log-line">
            {log.stamp && <span className="log-stamp">{log.stamp}</span>}
            {log.stamp && log.levelText ? ' ' : ''}
            {log.levelText && <span className={`log-level ${log.levelClass}`}>{log.levelText}</span>}
            {(log.body || log.service) && (
              <>
                <span> - </span>
                {log.service && <b>{log.service}</b>}
                {log.service && log.body ? ' ' : ''}
                <span>{log.body}</span>
              </>
            )}
          </div>
        ))
      )}
    </div>
  );
}
