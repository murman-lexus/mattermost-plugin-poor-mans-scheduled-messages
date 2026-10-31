declare module '*.css' { const value: string; export default value; }
interface Window {
    registerPlugin(id: string, plugin: unknown): void;
}
