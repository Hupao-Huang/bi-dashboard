import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import { API_BASE } from './config';
import reportWebVitals from './reportWebVitals';

const nativeFetch = window.fetch.bind(window);
window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof input === 'string'
    ? input
    : input instanceof Request
      ? input.url
      : String(input);

  if (!url.startsWith(API_BASE)) {
    return nativeFetch(input, init);
  }

  if (input instanceof Request) {
    const request = new Request(input, {
      ...init,
      credentials: init?.credentials ?? 'include',
    });
    return nativeFetch(request);
  }

  return nativeFetch(input, {
    ...init,
    credentials: init?.credentials ?? 'include',
  });
}) as typeof window.fetch;

const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
