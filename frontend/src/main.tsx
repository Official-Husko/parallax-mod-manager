import {render} from 'preact';
import {App} from './app';
import {installGlobalErrorLogging} from './data/appLog';
import './style.css';

installGlobalErrorLogging();
render(<App/>, document.getElementById('app')!);
