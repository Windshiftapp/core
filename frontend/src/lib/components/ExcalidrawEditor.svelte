<script>
  import { onMount, onDestroy } from 'svelte';
  import { mountReactComponent } from '../composables/react-wrapper.js';
  import React from 'react';
  import "@excalidraw/excalidraw/index.css";

  let { initialData = null, onChange = () => {}, theme = 'light' } = $props();

  let container;
  let excalidrawRef;
  let reactComponent;

  onMount(async () => {
    // Dynamically import Excalidraw to avoid SSR issues
    const { Excalidraw } = await import('@excalidraw/excalidraw');

    // Create React component
    const ExcalidrawComponent = (props) => {
      return React.createElement(Excalidraw, {
        theme: theme,
        excalidrawAPI: (api) => {
          excalidrawRef = api;
        },
        initialData: initialData || {
          elements: [],
          appState: {
            viewBackgroundColor: theme === 'dark' ? '#1e1e1e' : '#ffffff'
          },
          scrollToContent: true
        },
        onChange: (elements, appState, files) => {
          const sceneData = {
            elements: elements,
            appState: {
              viewBackgroundColor: appState.viewBackgroundColor,
              currentItemStrokeColor: appState.currentItemStrokeColor,
              currentItemBackgroundColor: appState.currentItemBackgroundColor,
              currentItemFillStyle: appState.currentItemFillStyle,
              currentItemStrokeWidth: appState.currentItemStrokeWidth,
              currentItemRoughness: appState.currentItemRoughness,
              currentItemOpacity: appState.currentItemOpacity,
              currentItemFontFamily: appState.currentItemFontFamily,
              currentItemFontSize: appState.currentItemFontSize,
              currentItemTextAlign: appState.currentItemTextAlign,
              currentItemStrokeStyle: appState.currentItemStrokeStyle,
              currentItemRoundness: appState.currentItemRoundness,
            },
            files: files || {}
          };
          onChange(sceneData);
        }
      });
    };

    // Mount React component
    reactComponent = mountReactComponent(container, ExcalidrawComponent);
  });

  onDestroy(() => {
    if (reactComponent) {
      reactComponent.destroy();
    }
  });

  export function getSceneData() {
    if (!excalidrawRef) return null;
    const elements = excalidrawRef.getSceneElements();
    const appState = excalidrawRef.getAppState();
    const files = excalidrawRef.getFiles();

    return {
      elements,
      appState: {
        viewBackgroundColor: appState.viewBackgroundColor,
      },
      files
    };
  }
</script>

<div bind:this={container} class="w-full h-full"></div>

<style>
  div {
    min-height: 500px;
  }
</style>
