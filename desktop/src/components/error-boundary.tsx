import { Component, type ReactNode } from "react";
import { Button } from "./ui/button";

interface Props {
  children: ReactNode;
  /// A change of resetKey clears a caught error, so moving to another item
  /// gets a fresh attempt without remounting what is around it.
  resetKey?: string;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidUpdate(previous: Props) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) this.setState({ error: null });
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div className="m-6 max-w-md space-y-3 rounded-lg border border-destructive/30 bg-destructive/8 p-4">
        <p className="text-sm font-medium text-destructive">This screen hit an unexpected error.</p>
        <p className="text-sm text-subtle-foreground select-text">{error.message}</p>
        <Button variant="outline" onClick={() => this.setState({ error: null })}>
          Retry
        </Button>
      </div>
    );
  }
}
