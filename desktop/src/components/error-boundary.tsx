import { Component, type ReactNode } from "react";
import { Button } from "./ui/button";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div className="max-w-md space-y-3 rounded-md border border-red-200 bg-red-50 p-4">
        <p className="text-sm font-medium text-red-800">This screen hit an unexpected error.</p>
        <p className="text-sm text-red-700">{error.message}</p>
        <Button variant="secondary" onClick={() => this.setState({ error: null })}>
          Retry
        </Button>
      </div>
    );
  }
}
