import React from 'react';
import { pluginBridge } from './plugin-bridge-client.js';

function App() {
  const handleShowModal = async () => {
    const result = await pluginBridge.showModal({
      title: 'Demo Modal from React Plugin',
      content: {
        message: 'This is a modal opened via the UI bridge!',
        details: 'The plugin can send structured content to display in the host application.'
      },
      maxWidth: 'max-w-2xl'
    });
    console.log('Modal closed with result:', result);
  };

  const handleShowConfirm = async () => {
    const confirmed = await pluginBridge.showConfirm({
      title: 'Confirm Action',
      message: 'Are you sure you want to proceed with this demo action?',
      confirmText: 'Yes, Continue',
      cancelText: 'Cancel',
      variant: 'primary'
    });

    if (confirmed) {
      pluginBridge.showToast('Action confirmed!', 'success');
    } else {
      pluginBridge.showToast('Action cancelled', 'info');
    }
  };

  const handleShowToast = () => {
    pluginBridge.showToast('Success! This is a toast notification from the React plugin.', 'success', 3000);
  };

  return (
    <div className="p-8 max-w-4xl mx-auto">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900 mb-2">React Demo Plugin</h1>
        <p className="text-gray-600">
          Demonstrating UI bridge integration with React 19 and Tailwind CSS
        </p>
      </div>

      <div className="bg-white rounded-lg shadow-md border border-gray-200 p-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">UI Bridge Demo</h2>
        <p className="text-gray-600 mb-6">
          Click the buttons below to test different UI bridge capabilities:
        </p>

        <div className="space-y-4">
          <div className="flex flex-col space-y-2">
            <button
              onClick={handleShowModal}
              className="w-full bg-blue-600 hover:bg-blue-700 text-white font-medium py-3 px-4 rounded-lg transition-colors duration-200 flex items-center justify-center"
            >
              <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
              Open Modal via UI Bridge
            </button>
            <p className="text-sm text-gray-500 pl-2">Shows a modal dialog in the host application</p>
          </div>

          <div className="flex flex-col space-y-2">
            <button
              onClick={handleShowConfirm}
              className="w-full bg-purple-600 hover:bg-purple-700 text-white font-medium py-3 px-4 rounded-lg transition-colors duration-200 flex items-center justify-center"
            >
              <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Open Confirm Dialog via UI Bridge
            </button>
            <p className="text-sm text-gray-500 pl-2">Shows a confirmation dialog with Yes/No options</p>
          </div>

          <div className="flex flex-col space-y-2">
            <button
              onClick={handleShowToast}
              className="w-full bg-green-600 hover:bg-green-700 text-white font-medium py-3 px-4 rounded-lg transition-colors duration-200 flex items-center justify-center"
            >
              <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Send Success Toast via UI Bridge
            </button>
            <p className="text-sm text-gray-500 pl-2">Displays a success notification toast</p>
          </div>
        </div>
      </div>

      <div className="mt-8 bg-gray-50 rounded-lg border border-gray-200 p-6">
        <h3 className="text-lg font-semibold text-gray-800 mb-3">Plugin Information</h3>
        <dl className="space-y-2 text-sm">
          <div className="flex">
            <dt className="font-medium text-gray-600 w-32">Framework:</dt>
            <dd className="text-gray-900">React 19</dd>
          </div>
          <div className="flex">
            <dt className="font-medium text-gray-600 w-32">Styling:</dt>
            <dd className="text-gray-900">Tailwind CSS</dd>
          </div>
          <div className="flex">
            <dt className="font-medium text-gray-600 w-32">Display Mode:</dt>
            <dd className="text-gray-900">iframe</dd>
          </div>
          <div className="flex">
            <dt className="font-medium text-gray-600 w-32">Integration:</dt>
            <dd className="text-gray-900">Plugin UI Bridge</dd>
          </div>
        </dl>
      </div>
    </div>
  );
}

export default App;
