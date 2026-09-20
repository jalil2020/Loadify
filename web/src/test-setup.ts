// jsdom has <dialog> but not its methods, so a component that opens a modal
// throws there. Every dialog in this app goes through components/ui.tsx, which
// calls showModal and close, so without this no test could open one.
//
// The stand-in only does what the real one does to the DOM: set and clear the
// open attribute, which is what `dialog.open` reflects, and fire `close`.
if (!HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.show = function show() {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function close(returnValue?: string) {
    if (returnValue !== undefined) this.returnValue = returnValue;
    this.removeAttribute("open");
    this.dispatchEvent(new Event("close"));
  };
}
