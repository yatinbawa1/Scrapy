export function click(node, handler) {
  function dispatch(event) {
    handler(event);
  }

  node.addEventListener('click', dispatch);

  return {
    update(newHandler) {
      handler = newHandler;
    },
    destroy() {
      node.removeEventListener('click', dispatch);
    }
  };
}
