import { useEffect, useState, useRef } from 'react';
import { Button } from '@/components/ds';
import { CloseOutlined, ArrowRightOutlined, ArrowLeftOutlined } from '@ant-design/icons';
interface GuideStep {
  selector: string;
  title: string;
  description: string;
  fallbackPosition?: { x: number; y: number };
}

interface TelemetryGuideOverlayProps {
  active: boolean;
  onClose: () => void;
  page: 'index' | 'inbounds';
}

const INDEX_STEPS: GuideStep[] = [
  {
    selector: '.cockpit-title-node',
    title: 'TELEMETRY MISSION CONTROL',
    description: 'This is the primary hardware telemetry module. Status indicators represent host machine state updates streamed via websockets.'
  },
  {
    selector: '.telemetry-dial-box',
    title: 'HARDWARE CIRCULAR DIALS',
    description: 'Minimal circular telemetry meters. Displays real-time CPU speed, memory buffers, swap memory, and storage volume allocations.'
  },
  {
    selector: '.ant-card:has(.ant-card-actions)',
    title: 'DIAGNOSTIC & LOG PANELS',
    description: 'Access real-time Xray service logs, system database backup configs, and the raw config.json core structures directly.'
  },
  {
    selector: '.ant-statistic:has(.anticon-thunderbolt)',
    title: 'SERVICE RUNTIME METRICS',
    description: 'Xray core process uptime vs OS system uptime metrics. Monitors thread counts and physical memory limits.'
  }
];

const INBOUNDS_STEPS: GuideStep[] = [
  {
    selector: '.inbounds-page .ant-table-container',
    title: 'COCKPIT CONSOLE GRID',
    description: 'Minimalist list displaying all active server inbound entrypoints. Monospaced columns ensure readable bandwidth counters.'
  },
  {
    selector: '.protocol-tags',
    title: 'MULTI-PROTOCOL BADGES',
    description: 'Active transport protocols (VMess, VLESS, Trojan, SS) rendered with unique pill design systems matching their protocol context.'
  },
  {
    selector: '.client-count-tag',
    title: 'ACTIVE TELEMETRY COUNTERS',
    description: 'Live active clients tracker. Hovering reveals specific user emails, active statuses, and depleted or online labels.'
  },
  {
    selector: '.inbounds-page .ant-btn-primary:first-child',
    title: 'INBOUND CONTROLLER',
    description: 'Create new configurations with customized ports, TLS settings, reality keys, and custom fallbacks.'
  }
];

export default function TelemetryGuideOverlay({ active, onClose, page }: TelemetryGuideOverlayProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const [targetRect, setTargetRect] = useState<DOMRect | null>(null);
  const steps = page === 'index' ? INDEX_STEPS : INBOUNDS_STEPS;
  const overlayRef = useRef<HTMLDivElement>(null);

  // Recalculate target element position
  const updateTargetPosition = () => {
    if (!active) return;
    const step = steps[currentStep];
    const element = document.querySelector(step.selector);
    if (element) {
      setTargetRect(element.getBoundingClientRect());
    } else {
      setTargetRect(null);
    }
  };

  useEffect(() => {
    updateTargetPosition();
    window.addEventListener('resize', updateTargetPosition);
    window.addEventListener('scroll', updateTargetPosition);
    return () => {
      window.removeEventListener('resize', updateTargetPosition);
      window.removeEventListener('scroll', updateTargetPosition);
    };
  }, [active, currentStep, page]);

  useEffect(() => {
    if (active) {
      setCurrentStep(0);
    }
  }, [active]);

  if (!active) return null;

  const stepInfo = steps[currentStep];
  
  // Calculate callout box layout parameters
  const getCalloutStyles = () => {
    if (!targetRect) {
      return { top: '35%', left: '50%', transform: 'translate(-50%, -50%)' };
    }
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    
    let top = targetRect.bottom + window.scrollY + 20;
    let left = targetRect.left + window.scrollX + (targetRect.width / 2) - 160;

    // Boundary checks
    if (left < 10) left = 10;
    if (left + 320 > viewportWidth) left = viewportWidth - 330;
    if (top + 180 > viewportHeight + window.scrollY) {
      // Show above target if not enough room below
      top = targetRect.top + window.scrollY - 190;
    }
    if (top < 10) top = 10;

    return { top: `${top}px`, left: `${left}px` };
  };

  // SVG lines from target center to callout box center
  const renderSVGConnector = () => {
    if (!targetRect || !overlayRef.current) return null;
    
    // Target center coordinates relative to viewport
    const targetCenterX = targetRect.left + targetRect.width / 2;
    const targetCenterY = targetRect.top + targetRect.height / 2;

    // Callout box position
    const boxStyles = getCalloutStyles();
    const boxTop = parseFloat(boxStyles.top) - window.scrollY;
    const boxLeft = parseFloat(boxStyles.left) - window.scrollX;
    const boxCenterX = boxLeft + 160;
    const boxCenterY = boxTop + 75;

    return (
      <svg className="guide-connector-svg" style={{ width: '100vw', height: '100vh' }}>
        {/* target pointer */}
        <circle 
          cx={targetCenterX} 
          cy={targetCenterY} 
          r="8" 
          fill="none" 
          stroke="#3279F9" 
          strokeWidth="1.5"
        />
        <circle 
          cx={targetCenterX} 
          cy={targetCenterY} 
          r="2" 
          fill="#3279F9" 
        />
        <line 
          x1={targetCenterX - 15} 
          y1={targetCenterY} 
          x2={targetCenterX + 15} 
          y2={targetCenterY} 
          stroke="#3279F9" 
          strokeWidth="0.8" 
          strokeDasharray="2,2"
        />
        <line 
          x1={targetCenterX} 
          y1={targetCenterY - 15} 
          x2={targetCenterX} 
          y2={targetCenterY + 15} 
          stroke="#3279F9" 
          strokeWidth="0.8" 
          strokeDasharray="2,2"
        />

        {/* Dashed connector line */}
        <path 
          d={`M ${targetCenterX} ${targetCenterY} L ${boxCenterX} ${boxCenterY}`} 
          fill="none" 
          stroke="#3279F9" 
          strokeWidth="1.5" 
          strokeDasharray="4,4"
        />
      </svg>
    );
  };

  return (
    <div className="telemetry-guide-overlay-root" ref={overlayRef}>
      <div className="guide-backdrop" onClick={onClose} />
      
      {/* Connector lines */}
      {renderSVGConnector()}

      {/* Target highlight mask */}
      {targetRect && (
        <div 
          className="guide-target-glow" 
          style={{
            top: `${targetRect.top + window.scrollY}px`,
            left: `${targetRect.left + window.scrollX}px`,
            width: `${targetRect.width}px`,
            height: `${targetRect.height}px`,
          }}
        />
      )}

      {/* HUD Callout Card */}
      <div className="guide-callout-card" style={getCalloutStyles()}>
        <div className="guide-card-header">
          <span className="guide-step-indicator">COCKPIT MANUAL // STEP {currentStep + 1} OF {steps.length}</span>
          <Button
            variant="text"
            size="sm"
            icon={<CloseOutlined style={{ color: '#3279F9' }} />}
            onClick={onClose}
          />
        </div>
        <div className="guide-card-body">
          <h4 className="guide-step-title">{stepInfo.title}</h4>
          <p className="guide-step-description">{stepInfo.description}</p>
        </div>
        <div className="guide-card-footer">
          <Button
            size="sm"
            variant="text"
            disabled={currentStep === 0}
            onClick={() => setCurrentStep(prev => prev - 1)}
            icon={<ArrowLeftOutlined />}
            className="guide-nav-btn"
          >
            PREV
          </Button>
          <div className="guide-dots">
            {steps.map((_, idx) => (
              <span 
                key={idx} 
                className={`guide-dot${idx === currentStep ? ' is-active' : ''}`}
                onClick={() => setCurrentStep(idx)}
              />
            ))}
          </div>
          {currentStep < steps.length - 1 ? (
            <Button
              size="sm"
              variant="text"
              onClick={() => setCurrentStep(prev => prev + 1)}
              icon={<ArrowRightOutlined />}
              className="guide-nav-btn"
            >
              NEXT
            </Button>
          ) : (
            <Button
              size="sm"
              variant="text"
              onClick={onClose}
              className="guide-nav-btn guide-finish-btn"
            >
              FINISH
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
