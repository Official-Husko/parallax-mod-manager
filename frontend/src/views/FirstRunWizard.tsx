import './FirstRunWizard.css';
import {h} from 'preact';
import {APP_NAME} from '../data/mockData';
import {detectedGames, wizardSteps} from '../data/mockData';

export function FirstRunWizard({onContinue}: { onContinue: () => void }) {
    return (
        <div className="overlay">
            <div className="wizard">
                <div className="wizard-sidebar">
                    <div className="wizard-icon"/>
                    <div className="wizard-title">Set up {APP_NAME}</div>
                    <div className="wizard-steps">
                        {wizardSteps.map((w) => (
                            <div key={w.n} className="wizard-step">
                                <span className="wizard-step-num" style={{borderColor: w.ringC, background: w.dotBg, color: w.numC}}>{w.n}</span>
                                <span style={{color: w.c}}>{w.label}</span>
                            </div>
                        ))}
                    </div>
                    <div className="wizard-footnote">Nothing is uploaded. Playset codes are only generated when you press Share.</div>
                </div>
                <div className="wizard-main">
                    <div className="wizard-heading">Games found</div>
                    <div className="wizard-subheading">Detected from your Steam library. Add anything installed elsewhere.</div>
                    <div className="wizard-detected-list">
                        {detectedGames.map((d) => (
                            <div key={d.name} className="wizard-detected-row" style={{borderColor: d.border, background: d.bg}}>
                                <span className="wizard-checkbox" style={{borderColor: d.boxC, background: d.boxBg}}/>
                                <div className="wizard-detected-swatch" style={{background: d.swatch}}/>
                                <span className="wizard-detected-name">{d.name}</span>
                                <span className="mono wizard-detected-mods">{d.mods}</span>
                            </div>
                        ))}
                    </div>
                    <div className="wizard-bottom">
                        <span className="btn-ghost">Browse manually...</span>
                        <div className="spacer"/>
                        <span className="wizard-back">Back</span>
                        <span className="btn-primary" onClick={onContinue}>Continue</span>
                    </div>
                </div>
            </div>
        </div>
    );
}
